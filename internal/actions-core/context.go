package ac

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// PayloadRepository mirrors the repository field in the webhook payload.
type PayloadRepository struct {
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Owner    struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	} `json:"owner"`
	HTMLURL string `json:"html_url"`
}

// IssuePayload mirrors the issue field in the webhook payload.
type IssuePayload struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

// PullRequestPayload mirrors the pull_request field in the webhook payload.
type PullRequestPayload struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Title   string `json:"title"`
}

// SenderPayload mirrors the sender field in the webhook payload.
type SenderPayload struct {
	Type string `json:"type"`
}

// InstallationPayload mirrors the installation field in the webhook payload.
type InstallationPayload struct {
	ID int `json:"id"`
}

// CommentPayload mirrors the comment field in the webhook payload.
type CommentPayload struct {
	ID int `json:"id"`
}

// HeadCommitPayload mirrors the head_commit field in push event payloads.
type HeadCommitPayload struct {
	Message string `json:"message"`
}

// WebhookPayload represents the parsed GitHub webhook event payload.
type WebhookPayload struct {
	Repository   *PayloadRepository   `json:"repository"`
	Issue        *IssuePayload        `json:"issue"`
	PullRequest  *PullRequestPayload  `json:"pull_request"`
	Sender       *SenderPayload       `json:"sender"`
	Action       string               `json:"action"`
	Installation *InstallationPayload `json:"installation"`
	Comment      *CommentPayload      `json:"comment"`
	HeadCommit   *HeadCommitPayload   `json:"head_commit"`
}

// Context holds the GitHub Actions workflow run context, mirroring the
// @actions/github Context class.
type Context struct {
	Payload    WebhookPayload
	EventName  string
	SHA        string
	Ref        string
	Workflow   string
	Action     string
	Actor      string
	Job        string
	RunAttempt int
	RunNumber  int
	RunID      int64
	APIURL     string
	ServerURL  string
	GraphQLURL string
}

// New reads GitHub Actions environment variables and returns a populated Context.
func NewContext() (*Context, error) {
	c := &Context{
		EventName:  os.Getenv("GITHUB_EVENT_NAME"),
		SHA:        os.Getenv("GITHUB_SHA"),
		Ref:        os.Getenv("GITHUB_REF"),
		Workflow:   os.Getenv("GITHUB_WORKFLOW"),
		Action:     os.Getenv("GITHUB_ACTION"),
		Actor:      os.Getenv("GITHUB_ACTOR"),
		Job:        os.Getenv("GITHUB_JOB"),
		APIURL:     envOrDefault("GITHUB_API_URL", "https://api.github.com"),
		ServerURL:  envOrDefault("GITHUB_SERVER_URL", "https://github.com"),
		GraphQLURL: envOrDefault("GITHUB_GRAPHQL_URL", "https://api.github.com/graphql"),
	}

	if v := os.Getenv("GITHUB_RUN_ATTEMPT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("parsing GITHUB_RUN_ATTEMPT: %w", err)
		}
		c.RunAttempt = n
	}

	if v := os.Getenv("GITHUB_RUN_NUMBER"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("parsing GITHUB_RUN_NUMBER: %w", err)
		}
		c.RunNumber = n
	}

	if v := os.Getenv("GITHUB_RUN_ID"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing GITHUB_RUN_ID: %w", err)
		}
		c.RunID = n
	}

	if path := os.Getenv("GITHUB_EVENT_PATH"); path != "" {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			fmt.Printf("GITHUB_EVENT_PATH %s does not exist\n", path)
		} else if err != nil {
			return nil, fmt.Errorf("reading GITHUB_EVENT_PATH: %w", err)
		} else {
			if err := json.Unmarshal(data, &c.Payload); err != nil {
				return nil, fmt.Errorf("parsing webhook payload: %w", err)
			}
		}
	}

	return c, nil
}

// RepoInfo holds the owner and repo name for the current repository.
type RepoInfo struct {
	Owner string
	Repo  string
}

// Repo returns the owner and repository name. It first checks the
// GITHUB_REPOSITORY env var, then falls back to the webhook payload.
func (c *Context) Repo() (RepoInfo, error) {
	if v := os.Getenv("GITHUB_REPOSITORY"); v != "" {
		parts := strings.SplitN(v, "/", 2)
		if len(parts) == 2 {
			return RepoInfo{Owner: parts[0], Repo: parts[1]}, nil
		}
	}

	if c.Payload.Repository != nil &&
		c.Payload.Repository.Owner.Login != "" &&
		c.Payload.Repository.Name != "" {
		return RepoInfo{
			Owner: c.Payload.Repository.Owner.Login,
			Repo:  c.Payload.Repository.Name,
		}, nil
	}

	return RepoInfo{}, fmt.Errorf("repository info unavailable: set GITHUB_REPOSITORY or ensure the event payload contains a repository field")
}

// IssueInfo holds the owner, repo, and issue/PR number.
type IssueInfo struct {
	Owner  string
	Repo   string
	Number int
}

// Issue returns the owner, repo, and issue or pull request number.
// Number is taken from the issue payload first, then the pull_request payload,
// defaulting to 0 if neither is present.
func (c *Context) Issue() (IssueInfo, error) {
	repo, err := c.Repo()
	if err != nil {
		return IssueInfo{}, err
	}

	number := 0
	if c.Payload.Issue != nil {
		number = c.Payload.Issue.Number
	} else if c.Payload.PullRequest != nil {
		number = c.Payload.PullRequest.Number
	}

	return IssueInfo{Owner: repo.Owner, Repo: repo.Repo, Number: number}, nil
}

// envOrDefault returns the value of the environment variable key, or fallback
// if the variable is unset or empty.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
