package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
)

func init() { Register("login", runLogin) }

type LoginInput struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func runLogin() error {
	inp := LoginInput{
		Registry: "ghcr.io",
	}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	// Fall back to GITHUB_ACTOR / GITHUB_TOKEN for GHCR
	if inp.Username == "" {
		inp.Username = os.Getenv("GITHUB_ACTOR")
		if inp.Username != "" {
			ac.Info(fmt.Sprintf("Auto-detected username: %s", inp.Username))
		}
	}
	if inp.Password == "" {
		inp.Password = os.Getenv("GITHUB_TOKEN")
		if inp.Password != "" {
			ac.Info("Auto-detected password from GITHUB_TOKEN")
		}
	}

	if inp.Username == "" {
		return fmt.Errorf("input 'username' is required (or set GITHUB_ACTOR)")
	}
	if inp.Password == "" {
		return fmt.Errorf("input 'password' is required (or set GITHUB_TOKEN)")
	}

	ac.SetSecret(inp.Password)

	ac.Info(fmt.Sprintf("Logging in to %s as %s", inp.Registry, inp.Username))

	args := []string{"login", inp.Registry, "-u", inp.Username, "--password-stdin"}
	ac.Info(fmt.Sprintf("Running: docker %s", strings.Join(args[:len(args)-1], " ")))

	c := exec.Command("docker", args...)
	c.Stdin = strings.NewReader(inp.Password)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("docker login failed: %w", err)
	}

	ac.Info(fmt.Sprintf("Logged in to %s", inp.Registry))
	return nil
}
