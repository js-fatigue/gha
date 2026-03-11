package cmd

import (
	"fmt"
	"regexp"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
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

// checkPRTitle validates a PR title against the conventional commit format.
// Returns a non-nil error with a human-readable message when the title is invalid.
func checkPRTitle(title string) error {
	if title == "" {
		return fmt.Errorf("pull request title is empty")
	}
	if !prTitleRE.MatchString(title) {
		return fmt.Errorf(
			"PR title %q does not follow conventional commit format.\n"+
				"Expected: `<type>[(<scope>)][!]: <description>`\n"+
				"Valid types: %s",
			title,
			strings.Join(conventionalTypes, ", "),
		)
	}
	return nil
}

func runCheckPRTitle() error {
	ctx, err := ac.NewContext()
	if err != nil {
		return fmt.Errorf("reading context: %w", err)
	}

	if ctx.Payload.PullRequest == nil {
		return fmt.Errorf("check-pr-title must run on a pull_request event (got %q)", ctx.EventName)
	}

	title := ctx.Payload.PullRequest.Title
	ac.Info(fmt.Sprintf("Checking PR title: %q", title))

	if err := checkPRTitle(title); err != nil {
		return err
	}

	ac.Info(fmt.Sprintf("PR title is valid: %q", title))
	return nil
}
