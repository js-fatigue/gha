package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
)

func init() { Register("assume-role", runAssumeRole) }

type AssumeRoleInput struct {
	Role        string `json:"role"`
	Region      string `json:"region"`
	SessionName string `json:"session_name"`
	Duration    int    `json:"duration"`
	Audience    string `json:"audience"`
}

// stsCredentials maps the inner <Credentials> element of the STS response.
type stsCredentials struct {
	AccessKeyID     string `xml:"AccessKeyId"`
	SecretAccessKey string `xml:"SecretAccessKey"`
	SessionToken    string `xml:"SessionToken"`
	Expiration      string `xml:"Expiration"`
}

// stsResponse is the envelope for AssumeRoleWithWebIdentity.
type stsResponse struct {
	XMLName     xml.Name       `xml:"AssumeRoleWithWebIdentityResponse"`
	Credentials stsCredentials `xml:"AssumeRoleWithWebIdentityResult>Credentials"`
}

// stsErrorResponse is the envelope for STS error responses.
type stsErrorResponse struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Code    string   `xml:"Error>Code"`
	Message string   `xml:"Error>Message"`
}

func runAssumeRole() error {
	inp := AssumeRoleInput{
		Region:   "us-east-1",
		Duration: 3600,
		Audience: "sts.amazonaws.com",
	}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	if inp.Role == "" {
		return fmt.Errorf("input 'role' is required (ARN of the IAM role to assume)")
	}

	// Auto-detect session name from GitHub context
	if inp.SessionName == "" {
		repo := os.Getenv("GITHUB_REPOSITORY")
		runID := os.Getenv("GITHUB_RUN_ID")
		name := strings.ReplaceAll(repo, "/", "-")
		if runID != "" {
			name = name + "@" + runID
		}
		if len(name) > 64 {
			name = name[:64]
		}
		if name == "" {
			name = "gha-assume-role"
		}
		inp.SessionName = name
		ac.Info(fmt.Sprintf("Auto-detected session name: %s", inp.SessionName))
	}

	ac.Info(fmt.Sprintf("Requesting OIDC token for audience: %s", inp.Audience))
	oidcToken, err := requestOIDCToken(inp.Audience)
	if err != nil {
		return fmt.Errorf("requesting OIDC token: %w", err)
	}

	ac.Info(fmt.Sprintf("Assuming role %s in region %s", inp.Role, inp.Region))
	creds, err := assumeRoleWithWebIdentity(inp, oidcToken)
	if err != nil {
		return fmt.Errorf("assuming role: %w", err)
	}

	ac.SetSecret(creds.AccessKeyID)
	ac.SetSecret(creds.SecretAccessKey)
	ac.SetSecret(creds.SessionToken)

	for k, v := range map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": creds.SecretAccessKey,
		"AWS_SESSION_TOKEN":     creds.SessionToken,
		"AWS_REGION":            inp.Region,
		"AWS_DEFAULT_REGION":    inp.Region,
	} {
		if err := ac.ExportVariable(k, v); err != nil {
			return fmt.Errorf("exporting %s: %w", k, err)
		}
	}

	if err := ac.SetOutput("aws_region", inp.Region); err != nil {
		ac.Warning(fmt.Sprintf("could not set aws_region output: %v", err), nil)
	}

	ac.Info(fmt.Sprintf("Credentials valid until %s", creds.Expiration))

	ac.JobSummary.AddHeading("AWS Assume Role", 2).AddTable([][]ac.SummaryTableCell{
		{
			{Data: "Role", Header: true},
			{Data: "Region", Header: true},
			{Data: "Session", Header: true},
			{Data: "Expires", Header: true},
		},
		{
			{Data: inp.Role},
			{Data: inp.Region},
			{Data: inp.SessionName},
			{Data: creds.Expiration},
		},
	})
	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}

// requestOIDCToken fetches a GitHub Actions OIDC JWT for the given audience.
func requestOIDCToken(audience string) (string, error) {
	reqURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	reqToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if reqURL == "" || reqToken == "" {
		return "", fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_URL / ACTIONS_ID_TOKEN_REQUEST_TOKEN not set; " +
			"ensure the workflow has 'id-token: write' permission")
	}

	u, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("parsing ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", audience)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+reqToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OIDC token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading OIDC token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC token request returned %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parsing OIDC token response: %w", err)
	}
	if payload.Value == "" {
		return "", fmt.Errorf("OIDC token response contained no value")
	}
	return payload.Value, nil
}

// assumeRoleWithWebIdentity calls the STS endpoint and returns the temporary credentials.
func assumeRoleWithWebIdentity(inp AssumeRoleInput, webIdentityToken string) (*stsCredentials, error) {
	endpoint := fmt.Sprintf("https://sts.%s.amazonaws.com/", inp.Region)

	form := url.Values{
		"Action":           {"AssumeRoleWithWebIdentity"},
		"Version":          {"2011-06-15"},
		"RoleArn":          {inp.Role},
		"RoleSessionName":  {inp.SessionName},
		"WebIdentityToken": {webIdentityToken},
		"DurationSeconds":  {fmt.Sprintf("%d", inp.Duration)},
	}

	resp, err := http.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode())) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("STS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading STS response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp stsErrorResponse
		if xmlErr := xml.Unmarshal(body, &errResp); xmlErr == nil && errResp.Code != "" {
			return nil, fmt.Errorf("STS error %s: %s", errResp.Code, errResp.Message)
		}
		return nil, fmt.Errorf("STS returned %d: %s", resp.StatusCode, body)
	}

	var result stsResponse
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing STS response: %w", err)
	}
	if result.Credentials.AccessKeyID == "" {
		return nil, fmt.Errorf("STS response contained no credentials")
	}
	return &result.Credentials, nil
}
