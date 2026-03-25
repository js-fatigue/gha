package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
)

func init() { Register("init", runInit) }

type InitInput struct {
	WorkingDirectory   string `json:"working_directory"    hcl:"working_directory"`
	Backend            bool   `json:"backend"              hcl:"backend"`
	Upgrade            bool   `json:"upgrade"              hcl:"upgrade"`
	Reconfigure        bool   `json:"reconfigure"          hcl:"reconfigure"`
	Lint               bool   `json:"lint"                 hcl:"lint"`
	LintFailOnFindings bool   `json:"lint_fail_on_findings" hcl:"lint_fail_on_findings"`
}

func runInit() error {
	inp := InitInput{
		Backend: true,
		Lint:    true,
	}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	if inp.WorkingDirectory != "" {
		if err := os.Chdir(inp.WorkingDirectory); err != nil {
			return fmt.Errorf("changing to working directory %q: %w", inp.WorkingDirectory, err)
		}
		ac.Info(fmt.Sprintf("Changed to working directory: %s", inp.WorkingDirectory))
	}

	if inp.Lint {
		hasFindings := runLinters()
		if hasFindings && inp.LintFailOnFindings {
			return fmt.Errorf("linting found issues; refusing to run terraform init")
		}
	}

	args := []string{"init", "-input=false"}
	if !inp.Backend {
		args = append(args, "-backend=false")
	}
	if inp.Upgrade {
		args = append(args, "-upgrade")
	}
	if inp.Reconfigure {
		args = append(args, "-reconfigure")
	}

	ac.Info(fmt.Sprintf("Running: terraform %s", strings.Join(args, " ")))

	res, err := ac.Exec("terraform", args...)
	if res.Stdout != "" {
		ac.Info(res.Stdout)
	}
	if res.Stderr != "" {
		ac.Info(res.Stderr)
	}
	if err != nil {
		return fmt.Errorf("terraform init failed: %w", err)
	}

	ac.Info("terraform init succeeded")
	return nil
}

// runLinters runs tflint and checkov in the CWD. Returns true if either tool reported findings.
func runLinters() bool {
	hasFindings := false
	hasFindings = runLinter("tflint", []string{}) || hasFindings
	hasFindings = runLinter("checkov", []string{"-d", ".", "--quiet", "--compact"}) || hasFindings
	return hasFindings
}

func runLinter(tool string, args []string) bool {
	if _, err := exec.LookPath(tool); err != nil {
		ac.Warning(fmt.Sprintf("%s not found in PATH, skipping", tool), nil)
		return false
	}

	ac.Info(fmt.Sprintf("Running: %s %s", tool, strings.Join(args, " ")))
	res, err := ac.Exec(tool, args...)
	if res.Stdout != "" {
		ac.Info(res.Stdout)
	}
	if res.Stderr != "" {
		ac.Info(res.Stderr)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			ac.Warning(fmt.Sprintf("%s exited %d — findings above", tool, exitErr.ExitCode()), nil)
			return true
		}
		ac.Warning(fmt.Sprintf("%s could not be run: %v", tool, err), nil)
		return false
	}

	ac.Info(fmt.Sprintf("%s: no issues found", tool))
	return false
}
