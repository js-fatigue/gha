package ac

import (
	"strings"
	"testing"
)

func TestExec(t *testing.T) {
	t.Run("captures stdout", func(t *testing.T) {
		res, err := Exec("echo", "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(res.Stdout) != "hello" {
			t.Errorf("stdout: got %q, want %q", res.Stdout, "hello\n")
		}
		if res.Stderr != "" {
			t.Errorf("stderr: got %q, want empty", res.Stderr)
		}
		if res.ExitCode != 0 {
			t.Errorf("exit code: got %d, want 0", res.ExitCode)
		}
	})

	t.Run("non-zero exit sets ExitCode and returns error", func(t *testing.T) {
		res, err := Exec("sh", "-c", "exit 1")
		if err == nil {
			t.Fatal("expected error for exit 1, got nil")
		}
		if res.ExitCode != 1 {
			t.Errorf("exit code: got %d, want 1", res.ExitCode)
		}
	})

	t.Run("captures stdout and stderr separately", func(t *testing.T) {
		res, err := Exec("sh", "-c", "echo out; echo err >&2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(res.Stdout) != "out" {
			t.Errorf("stdout: got %q, want %q", res.Stdout, "out\n")
		}
		if strings.TrimSpace(res.Stderr) != "err" {
			t.Errorf("stderr: got %q, want %q", res.Stderr, "err\n")
		}
	})

	t.Run("command not found returns error", func(t *testing.T) {
		_, err := Exec("this-command-does-not-exist-anywhere")
		if err == nil {
			t.Fatal("expected error for missing command, got nil")
		}
	})
}
