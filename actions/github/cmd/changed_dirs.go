package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"

	ac "github.com/bshore/gha/internal/actions-core"
)

type ChangedDirsInput struct {
	Base     string `json:"base"`
	MaxDepth int    `json:"max_depth"`
}

func init() { Register("changed-dirs", runChangedDirs) }

// defaultBase returns the best guess for the default branch by inspecting
// the local git remote tracking refs. Falls back to "main".
func defaultBase() string {
	// Prefer origin/HEAD which git sets when the remote explicitly advertises it.
	out, err := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		if b := strings.TrimPrefix(ref, "refs/remotes/origin/"); b != ref {
			return b
		}
	}
	// Fall back to checking common names locally.
	for _, candidate := range []string{"main", "master"} {
		if exec.Command("git", "rev-parse", "--verify", "--quiet", "origin/"+candidate).Run() == nil {
			return candidate
		}
	}
	return "main"
}

func runChangedDirs() error {
	inp := ChangedDirsInput{}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}
	if inp.Base == "" {
		inp.Base = defaultBase()
		ac.Info(fmt.Sprintf("Auto-detected base branch: %s", inp.Base))
	}
	base := inp.Base
	maxDepth := inp.MaxDepth

	// Resolve base ref: prefer local (e.g. when the branch is checked out),
	// fall back to origin/<base> (the common case on CI runners where only the
	// feature branch is checked out and base is a remote-tracking ref only).
	resolvedBase := base
	if exec.Command("git", "rev-parse", "--verify", base).Run() != nil {
		resolvedBase = "origin/" + base
	}

	// Get changed files via three-dot diff (compares merge base to HEAD,
	// so diverged branches still produce the correct feature-branch diff).
	out, err := exec.Command("git", "diff", "--name-only", resolvedBase+"...HEAD").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return fmt.Errorf("git diff %s...HEAD: %w\n%s", resolvedBase, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return fmt.Errorf("git diff %s...HEAD: %w", resolvedBase, err)
	}

	// Detect whether HEAD is missing commits from base (covers both pure-behind
	// and diverged cases). Count commits reachable from base but not from HEAD;
	// any count > 0 means the branch needs a rebase.
	behindOut, _ := exec.Command("git", "rev-list", "--count", "HEAD.."+resolvedBase).Output()
	isBehind := strings.TrimSpace(string(behindOut)) != "0"

	// Parse output into unique top-level directories.
	dirSet := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			dirSet[capDir(path.Dir(line), maxDepth)] = struct{}{}
		}
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	dirNamesJSON, _ := json.Marshal(dirs)

	// Set output before any early return so it is always populated.
	if err := ac.SetOutput("dir_names", string(dirNamesJSON)); err != nil {
		ac.Warning(fmt.Sprintf("could not set output: %v", err), nil)
	}

	// Build summary.
	status := "ahead"
	statusEmoji := "✅"
	if isBehind {
		status = "behind"
		statusEmoji = "❌"
	} else if len(dirs) == 0 {
		status = "identical"
		statusEmoji = "➖"
	}

	ac.JobSummary.
		AddHeading("Changed Directories", 2).
		AddTable([][]ac.SummaryTableCell{
			{
				{Data: "Base", Header: true},
				{Data: "Status", Header: true},
			},
			{
				{Data: base},
				{Data: statusEmoji + " " + status},
			},
		}).
		AddSeparator()

	if len(dirs) > 0 {
		ac.JobSummary.AddList(dirs, false)
	} else {
		ac.JobSummary.AddRaw("_No changed directories detected._", true)
	}

	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	if isBehind {
		return fmt.Errorf("HEAD is behind base %q; rebase the branch and retry", base)
	}

	return nil
}

func capDir(dir string, maxDepth int) string {
	if maxDepth <= 0 || dir == "." {
		return dir
	}
	parts := strings.Split(dir, "/")
	if len(parts) <= maxDepth {
		return dir
	}
	return strings.Join(parts[:maxDepth], "/")
}
