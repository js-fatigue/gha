package cmd

import (
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("changed-dirs", runChangedDirs) }

func runChangedDirs() error {
	base, _ := ac.GetInput("base", nil)
	if base == "" {
		base = "main"
	}

	maxDepth := 0
	if raw, _ := ac.GetInput("max_depth", nil); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxDepth = n
		}
	}

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

	// Detect whether HEAD is behind base (HEAD is an ancestor of base).
	// git merge-base --is-ancestor A B exits 0 if A is an ancestor of B.
	isBehind := exec.Command("git", "merge-base", "--is-ancestor", "HEAD", resolvedBase).Run() == nil

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
	dirNames := strings.Join(dirs, " ")

	// Set output before any early return so it is always populated.
	if err := ac.SetOutput("dir_names", dirNames); err != nil {
		ac.Warning(fmt.Sprintf("could not set output: %v", err), nil)
	}

	// Build summary.
	status := "ahead"
	statusEmoji := "✅"
	if isBehind {
		status = "behind"
		statusEmoji = "❌"
	} else if dirNames == "" {
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
