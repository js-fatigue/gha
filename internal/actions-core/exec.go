package ac

import (
	"bytes"
	"os/exec"
)

// ExecResult holds the captured output of a completed command.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int // 0 on success
}

// Exec runs name with args, capturing stdout and stderr separately.
// On a non-zero exit, err is a *exec.ExitError and ExitCode is set.
func Exec(name string, args ...string) (ExecResult, error) {
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	res := ExecResult{Stdout: outBuf.String(), Stderr: errBuf.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
	}
	return res, err
}
