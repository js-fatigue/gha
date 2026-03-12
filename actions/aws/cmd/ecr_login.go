package cmd

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
)

func init() { Register("ecr-login", runECRLogin) }

type ECRLoginInput struct {
	Registry string `json:"registry"`
	Region   string `json:"region"`
}

func runECRLogin() error {
	inp := ECRLoginInput{}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	if inp.Registry == "" {
		return fmt.Errorf("input 'registry' is required (ECR registry URI, e.g. 123456789012.dkr.ecr.us-east-2.amazonaws.com/repo)")
	}

	// Normalize: strip scheme and path, keeping only the hostname.
	registryHost, err := parseECRHost(inp.Registry)
	if err != nil {
		return fmt.Errorf("parsing registry: %w", err)
	}

	// Mask the AWS account ID embedded in the registry hostname.
	if accountID := accountIDFromECRHost(registryHost); accountID != "" {
		ac.SetSecret(accountID)
	}

	// Auto-detect region from the ECR hostname, fall back to AWS_REGION.
	if inp.Region == "" {
		inp.Region = regionFromECRHost(registryHost)
		if inp.Region != "" {
			ac.Info(fmt.Sprintf("Auto-detected region from registry hostname: %s", inp.Region))
		}
	}
	if inp.Region == "" {
		inp.Region = os.Getenv("AWS_REGION")
		if inp.Region != "" {
			ac.Info(fmt.Sprintf("Auto-detected region from AWS_REGION: %s", inp.Region))
		}
	}
	if inp.Region == "" {
		return fmt.Errorf("could not determine AWS region; set 'region' in input or run assume-role first")
	}

	ac.Info(fmt.Sprintf("Fetching ECR token for %s (region: %s)", registryHost, inp.Region))

	getPass := exec.Command("aws", "ecr", "get-login-password", "--region", inp.Region)
	getPass.Stderr = os.Stderr
	password, err := getPass.Output()
	if err != nil {
		return fmt.Errorf("aws ecr get-login-password: %w", err)
	}

	passwordStr := strings.TrimSpace(string(password))
	ac.SetSecret(passwordStr)

	// Run docker login with stderr captured — mirrors docker/login-action's
	// "silent: true" approach, which discards the "unencrypted" warning on success.
	var stderr bytes.Buffer
	login := exec.Command("docker", "login", registryHost, "-u", "AWS", "--password-stdin")
	login.Stdin = strings.NewReader(passwordStr)
	login.Stdout = os.Stdout
	login.Stderr = &stderr
	if err := login.Run(); err != nil {
		return fmt.Errorf("docker login: %w\n%s", err, stderr.String())
	}

	ac.Info(fmt.Sprintf("Logged in to %s", registryHost))

	if err := ac.SetOutput("registry", registryHost); err != nil {
		ac.Warning(fmt.Sprintf("could not set registry output: %v", err), nil)
	}

	return nil
}

// parseECRHost strips the scheme and path from a registry URI, returning
// just the hostname. Accepts both bare hostnames and full URIs.
func parseECRHost(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("could not parse hostname from %q", raw)
	}
	return host, nil
}

// accountIDFromECRHost extracts the AWS account ID (the first label) from a
// hostname of the form <account>.dkr.ecr.<region>.amazonaws.com.
func accountIDFromECRHost(host string) string {
	parts := strings.SplitN(host, ".dkr.ecr.", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

// regionFromECRHost extracts the AWS region from a hostname of the form
// <account>.dkr.ecr.<region>.amazonaws.com.
func regionFromECRHost(host string) string {
	parts := strings.SplitN(host, ".dkr.ecr.", 2)
	if len(parts) != 2 {
		return ""
	}
	// parts[1] = "us-east-2.amazonaws.com"
	return strings.TrimSuffix(parts[1], ".amazonaws.com")
}
