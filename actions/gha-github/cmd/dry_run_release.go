package cmd

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v68/github"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("dry-run-release", runDryRunRelease) }

var conventionalCommitRE = regexp.MustCompile(
	`^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\([^)]+\))?(!)?: `,
)

func runDryRunRelease() error {
	actionDir, _ := ac.GetInput("action_dir", nil)
	if actionDir == "" {
		return fmt.Errorf("action_dir input is required for dry-run-release")
	}
	familyName := path.Base(actionDir) // e.g. "actions/gha-github" → "gha-github"

	ctx, err := ac.NewContext()
	if err != nil {
		return fmt.Errorf("reading context: %w", err)
	}

	if ctx.Payload.PullRequest == nil {
		return fmt.Errorf("dry-run-release must run on a pull_request event (got %q)", ctx.EventName)
	}

	title := ctx.Payload.PullRequest.Title
	ac.Info(fmt.Sprintf("PR title: %q", title))
	ac.Info(fmt.Sprintf("Action family: %q", familyName))

	bumpType := determineBumpType(title)
	ac.Info(fmt.Sprintf("Bump type: %s", bumpType))

	client, err := ac.NewClient()
	if err != nil {
		return fmt.Errorf("creating GitHub client: %w", err)
	}

	repoInfo, err := ctx.Repo()
	if err != nil {
		return fmt.Errorf("reading repo info: %w", err)
	}
	tagPrefix := fmt.Sprintf("%s-v", familyName)
	currentVersion, currentTag := findLatestVersion(client, repoInfo.Owner, repoInfo.Repo, tagPrefix)
	ac.Info(fmt.Sprintf("Current version: %s (tag: %q)", currentVersion, currentTag))

	nextVersion := applyBump(currentVersion, bumpType)
	nextTag := tagPrefix + nextVersion
	ac.Info(fmt.Sprintf("Next tag: %s", nextTag))

	currentTagDisplay := currentTag
	if currentTagDisplay == "" {
		currentTagDisplay = "_none_"
	}

	// Write step summary.
	ac.JobSummary.
		AddHeading(fmt.Sprintf("Dry-Run Release: `%s`", familyName), 2).
		AddTable([][]ac.SummaryTableCell{
			{
				{Data: "PR Title", Header: true},
				{Data: "Bump Type", Header: true},
				{Data: "Current Tag", Header: true},
				{Data: "Next Tag", Header: true},
			},
			{
				{Data: title},
				{Data: bumpType},
				{Data: currentTagDisplay},
				{Data: nextTag},
			},
		})

	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	// Upsert PR comment with family-specific marker.
	marker := fmt.Sprintf("<!-- gha-dry-run-release-%s -->", familyName)
	var sb strings.Builder
	sb.WriteString(marker + "\n")
	fmt.Fprintf(&sb, "## Dry-Run Release: `%s`\n\n", familyName)
	sb.WriteString("| PR Title | Bump Type | Current Tag | Next Tag |\n")
	sb.WriteString("|---|---|---|---|\n")
	fmt.Fprintf(&sb, "| %s | %s | %s | `%s` |\n", title, bumpType, currentTagDisplay, nextTag)
	ac.UpsertPRComment(ctx, marker, sb.String())

	return nil
}

// determineBumpType returns "major", "minor", or "patch" based on the PR title.
func determineBumpType(title string) string {
	m := conventionalCommitRE.FindStringSubmatch(title)
	if m == nil {
		return "patch"
	}
	// m[3] is the "!" capture group.
	if m[3] == "!" {
		return "major"
	}
	if m[1] == "feat" {
		return "minor"
	}
	return "patch"
}

type semver struct{ major, minor, patch int }

// findLatestVersion lists releases filtered by tagPrefix, parses semver, and
// returns the highest version found. Returns ("0.0.0", "") if none exist.
func findLatestVersion(client *github.Client, owner, repo, tagPrefix string) (string, string) {
	opts := &github.ListOptions{PerPage: 100}
	best := semver{}
	bestTag := ""

	for {
		releases, resp, err := client.Repositories.ListReleases(context.Background(), owner, repo, opts)
		if err != nil {
			ac.Warning(fmt.Sprintf("listing releases: %v", err), nil)
			break
		}
		for _, r := range releases {
			tag := r.GetTagName()
			if !strings.HasPrefix(tag, tagPrefix) {
				continue
			}
			versionStr := strings.TrimPrefix(tag, tagPrefix)
			parts := strings.SplitN(versionStr, ".", 3)
			if len(parts) != 3 {
				continue
			}
			major, err1 := strconv.Atoi(parts[0])
			minor, err2 := strconv.Atoi(parts[1])
			patch, err3 := strconv.Atoi(parts[2])
			if err1 != nil || err2 != nil || err3 != nil {
				continue
			}
			v := semver{major, minor, patch}
			if v.major > best.major ||
				(v.major == best.major && v.minor > best.minor) ||
				(v.major == best.major && v.minor == best.minor && v.patch > best.patch) {
				best = v
				bestTag = tag
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return fmt.Sprintf("%d.%d.%d", best.major, best.minor, best.patch), bestTag
}

// applyBump increments the appropriate component of a "MAJOR.MINOR.PATCH" string.
func applyBump(version, bumpType string) string {
	parts := strings.SplitN(version, ".", 3)
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])

	switch bumpType {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	default: // patch
		patch++
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}
