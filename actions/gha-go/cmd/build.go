package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("build", runBuild) }

func runBuild() error {
	workingDir, _ := ac.GetInput("working_directory", nil)
	if workingDir == "" {
		workingDir = "."
	}
	output, _ := ac.GetInput("output", nil)
	ldflags, _ := ac.GetInput("ldflags", nil)
	cgoEnabled, _ := ac.GetInput("cgo_enabled", nil)
	if cgoEnabled == "" {
		cgoEnabled = "0"
	}

	args := []string{"build"}
	if output != "" {
		args = append(args, "-o", output)
	}
	if ldflags != "" {
		args = append(args, fmt.Sprintf("-ldflags=%s", ldflags))
	}
	args = append(args, workingDir)

	ac.Info(fmt.Sprintf("Running: go %s", strings.Join(args, " ")))

	c := exec.Command("go", args...)
	c.Env = append(os.Environ(), fmt.Sprintf("CGO_ENABLED=%s", cgoEnabled))
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build failed: %w\n%s", err, out)
	}

	ac.Info("Build succeeded")
	if output != "" {
		if err := ac.SetOutput("binary_path", output); err != nil {
			ac.Warning(fmt.Sprintf("could not set binary_path output: %v", err), nil)
		}
	}

	outputDisplay := "(default)"
	if output != "" {
		outputDisplay = output
	}
	ac.JobSummary.
		AddHeading("Go Build", 2).
		AddTable([][]ac.SummaryTableCell{
			{
				{Data: "Package", Header: true},
				{Data: "Output", Header: true},
				{Data: "CGO", Header: true},
				{Data: "Result", Header: true},
			},
			{
				{Data: workingDir},
				{Data: outputDisplay},
				{Data: cgoEnabled},
				{Data: "✅ success"},
			},
		})
	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}
