package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("build", runBuild) }

type BuildInput struct {
	WorkingDirectory string `json:"working_directory"`
	Output           string `json:"output"`
	LDFlags          string `json:"ldflags"`
	CGOEnabled       string `json:"cgo_enabled"`
}

func runBuild() error {
	inp := BuildInput{
		WorkingDirectory: ".",
		CGOEnabled:       "0",
	}
	if err := ac.GetJSONInput("build_input", &inp); err != nil {
		return fmt.Errorf("parsing build_input: %w", err)
	}
	workingDir := inp.WorkingDirectory
	output := inp.Output
	ldflags := inp.LDFlags
	cgoEnabled := inp.CGOEnabled

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
