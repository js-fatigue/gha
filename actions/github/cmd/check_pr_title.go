package cmd

import (
	"fmt"
	"regexp"
	"strings"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("check-pr-title", runCheckPRTitle) }

var (
	conventionalTypes = []string{
		"feat", "fix", "docs", "style", "refactor",
		"perf", "test", "chore", "ci", "build", "revert",
	}
	prTitleRE = regexp.MustCompile(
		`^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\([^)]+\))?(!)?:\s.+$`,
	)
)

func runCheckPRTitle() error {
	ctx, err := ac.NewContext()
	if err != nil {
		return fmt.Errorf("reading context: %w", err)
	}

	if ctx.Payload.PullRequest == nil {
		return fmt.Errorf("check-pr-title must run on a pull_request event (got %q)", ctx.EventName)
	}

	title := ctx.Payload.PullRequest.Title
	if title == "" {
		return fmt.Errorf("pull request title is empty")
	}

	ac.Info(fmt.Sprintf("Checking PR title: %q", title))

	if !prTitleRE.MatchString(title) {
		return fmt.Errorf(
			"PR title %q does not follow conventional commit format.\n"+
				"Expected: `<type>[(<scope>)][!]: <description>`\n"+
				"Valid types: %s",
			title,
			strings.Join(conventionalTypes, ", "),
		)
	}

	ac.Info(fmt.Sprintf("PR title is valid: %q", title))
	return nil
}
