package ac

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v68/github"
)

// New returns an authenticated GitHub client using credentials from the
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

	client := github.NewClient(nil).WithAuthToken(token)

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
