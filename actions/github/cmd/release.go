package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v84/github"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("release", runRelease) }

type ReleaseInput struct {
	ActionDir string `json:"action_dir"`
	Release   bool   `json:"release"`
}

var conventionalCommitRE = regexp.MustCompile(
	`^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\([^)]+\))?(!)?: `,
)

func runRelease() error {
	var inp ReleaseInput
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}
	if inp.ActionDir == "" {
		return fmt.Errorf("input.action_dir is required")
	}
	actionDir := inp.ActionDir
	doRelease := inp.Release
	familyName := path.Base(actionDir) // e.g. "actions/github" → "github"

	ctx, err := ac.NewContext()
	if err != nil {
		return fmt.Errorf("reading context: %w", err)
	}

	commitTitle := strings.SplitN(ctx.Payload.HeadCommit.Message, "\n", 2)[0]
	if ctx.Payload.PullRequest != nil {
		commitTitle = ctx.Payload.PullRequest.Title
	}

	ac.Info(fmt.Sprintf("Commit title: %q", commitTitle))
	ac.Info(fmt.Sprintf("Action family: %q", familyName))

	bumpType := determineBumpType(commitTitle)
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

	if doRelease {
		// Push the git tag first so the tag-push event triggers the release
		// asset workflow. Releases created solely via the REST API do not fire
		// push-tag events and therefore would never trigger release.yml.
		if out, err := exec.Command("git", "tag", nextTag).CombinedOutput(); err != nil {
			return fmt.Errorf("creating git tag %s: %w\n%s", nextTag, err, out)
		}
		if out, err := exec.Command("git", "push", "origin", nextTag).CombinedOutput(); err != nil {
			return fmt.Errorf("pushing git tag %s: %w\n%s", nextTag, err, out)
		}
		ac.Info(fmt.Sprintf("Pushed tag: %s", nextTag))

		// Create the GitHub release against the now-existing tag.
		releaseBody := fmt.Sprintf("Bump type: %s\n\nTriggered by: %s", bumpType, commitTitle)
		_, _, err := client.Repositories.CreateRelease(
			context.Background(),
			repoInfo.Owner,
			repoInfo.Repo,
			&github.RepositoryRelease{
				TagName: github.Ptr(nextTag),
				Name:    github.Ptr(nextTag),
				Body:    github.Ptr(releaseBody),
			},
		)
		if err != nil {
			return fmt.Errorf("creating release %s: %w", nextTag, err)
		}
		ac.Info(fmt.Sprintf("Created release: %s", nextTag))
		if err := ac.SetOutput("next_tag", nextTag); err != nil {
			ac.Warning(fmt.Sprintf("could not set next_tag output: %v", err), nil)
		}

		// Write step summary.
		ac.JobSummary.
			AddHeading(fmt.Sprintf("Release: `%s`", familyName), 2).
			AddTable([][]ac.SummaryTableCell{
				{
					{Data: "Commit Title", Header: true},
					{Data: "Bump Type", Header: true},
					{Data: "Previous Tag", Header: true},
					{Data: "Released Tag", Header: true},
				},
				{
					{Data: commitTitle},
					{Data: bumpType},
					{Data: currentTagDisplay},
					{Data: nextTag},
				},
			})

		if err := ac.JobSummary.Write(nil); err != nil {
			ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
		}
	} else {
		// Dry-run: write summary and upsert PR comment.
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
					{Data: commitTitle},
					{Data: bumpType},
					{Data: currentTagDisplay},
					{Data: nextTag},
				},
			})

		if err := ac.JobSummary.Write(nil); err != nil {
			ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
		}

		// Upsert PR comment with family-specific marker.
		marker := fmt.Sprintf("<!-- gha-release-%s -->", familyName)
		var sb strings.Builder
		sb.WriteString(marker + "\n")
		fmt.Fprintf(&sb, "## Dry-Run Release: `%s`\n\n", familyName)
		sb.WriteString("| PR Title | Bump Type | Current Tag | Next Tag |\n")
		sb.WriteString("|---|---|---|---|\n")
		fmt.Fprintf(&sb, "| %s | %s | %s | `%s` |\n", commitTitle, bumpType, currentTagDisplay, nextTag)
		ac.UpsertPRComment(ctx, marker, sb.String())
	}

	return nil
}

// determineBumpType returns "major", "minor", or "patch" based on the commit title.
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
