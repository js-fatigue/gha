package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
)

func init() { Register("build", runBuild) }

type BuildInput struct {
	WorkingDirectory string `json:"working_directory"`
	Output           string `json:"output"`
	LDFlags          string `json:"ldflags"`
	CGOEnabled       string `json:"cgo_enabled"`
	GOOS             string `json:"goos"`
	GOARCH           string `json:"goarch"`
	Trimpath         bool   `json:"trimpath"`
}

func runBuild() error {
	inp := BuildInput{
		WorkingDirectory: ".",
		CGOEnabled:       "0",
	}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
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
	if inp.Trimpath {
		args = append(args, "-trimpath")
	}
	args = append(args, workingDir)

	ac.Info(fmt.Sprintf("Running: go %s", strings.Join(args, " ")))

	// Go build command uses a command builder to set build environment vars and
	// cannot use the ac.Exec helper.
	c := exec.Command("go", args...)
	env := append(os.Environ(), fmt.Sprintf("CGO_ENABLED=%s", cgoEnabled))
	if inp.GOOS != "" {
		env = append(env, fmt.Sprintf("GOOS=%s", inp.GOOS))
	}
	if inp.GOARCH != "" {
		env = append(env, fmt.Sprintf("GOARCH=%s", inp.GOARCH))
	}
	c.Env = env
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
	goosDisplay := inp.GOOS
	if goosDisplay == "" {
		goosDisplay = "(native)"
	}
	goarchDisplay := inp.GOARCH
	if goarchDisplay == "" {
		goarchDisplay = "(native)"
	}
	trimpathDisplay := "false"
	if inp.Trimpath {
		trimpathDisplay = "true"
	}
	ac.JobSummary.AddHeading("Go Build", 2).AddTable([][]ac.SummaryTableCell{
		{
			{Data: "Package", Header: true},
			{Data: "Output", Header: true},
			{Data: "CGO", Header: true},
			{Data: "GOOS", Header: true},
			{Data: "GOARCH", Header: true},
			{Data: "Trimpath", Header: true},
			{Data: "Result", Header: true},
		},
		{
			{Data: workingDir},
			{Data: outputDisplay},
			{Data: cgoEnabled},
			{Data: goosDisplay},
			{Data: goarchDisplay},
			{Data: trimpathDisplay},
			{Data: "✅ success"},
		},
	})
	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}
