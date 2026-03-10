package ac

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v84/github"
)

type retryTransport struct {
	base        http.RoundTripper
	maxRetries  int
	exemptCodes map[int]struct{}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}
		}
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			if attempt < t.maxRetries && (req.Body == nil || req.GetBody != nil) {
				continue
			}
			return nil, err
		}
		_, exempt := t.exemptCodes[resp.StatusCode]
		if exempt || resp.StatusCode < 500 {
			return resp, nil
		}
		if attempt < t.maxRetries {
			if req.Body != nil && req.GetBody == nil {
				return resp, nil // body consumed, can't retry
			}
			resp.Body.Close()
			continue
		}
		return resp, nil
	}
	panic("retryTransport: loop overflow")
}

// NewClient returns an authenticated GitHub client using credentials from the
// GitHub Actions runner environment. Returns an error if GITHUB_TOKEN is unset
// or if URL parsing fails (GHES only).
func NewClient() (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is not set")
	}

	apiURL := os.Getenv("GITHUB_API_URL")
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}

	httpClient := &http.Client{
		Transport: &retryTransport{
			base:        http.DefaultTransport,
			maxRetries:  3,
			exemptCodes: map[int]struct{}{400: {}, 401: {}, 403: {}, 404: {}, 422: {}},
		},
	}
	client := github.NewClient(httpClient).WithAuthToken(token)

	if apiURL != "https://api.github.com" {
		// GHES: derive upload URL from API URL
		// e.g. https://github.example.com/api/v3 → https://github.example.com/api/uploads
		base := strings.TrimSuffix(apiURL, "/")
		uploadURL := strings.TrimSuffix(base, "v3") + "uploads"
		var err error
		client, err = client.WithEnterpriseURLs(base+"/", uploadURL+"/")
		if err != nil {
			return nil, fmt.Errorf("configuring enterprise GitHub client: %w", err)
		}
	}

	return client, nil
}
