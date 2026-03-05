package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bshore/gha/actions/github/cmd"
	ac "github.com/bshore/gha/internal/actions-core"
)

const prCommentMarker = "<!-- github-error -->"

func main() {
	defer ac.Exit()

	if len(os.Args) < 2 {
		ac.SetFailed("usage: github <command>")
		return
	}
	command := os.Args[1]

	ctx, err := ac.NewContext()
	if err != nil {
		ac.SetFailed(fmt.Sprintf("failed to read context: %v", err))
		return
	}

	if err := cmd.Dispatch(command); err != nil {
		ac.SetFailed(err.Error())
		ac.UpsertPRComment(ctx, prCommentMarker, buildErrorCommentBody(command, err))
	} else {
		ac.DeletePRComment(ctx, prCommentMarker)
	}

	if command != "cache" {
		ac.SelfCacheBinary()
	}
}

func buildErrorCommentBody(command string, cmdErr error) string {
	var sb strings.Builder
	sb.WriteString(prCommentMarker + "\n")
	fmt.Fprintf(&sb, "## GitHub Actions `%s` Failed\n\n", command)
	fmt.Fprintf(&sb, "**Error:** %s\n", cmdErr.Error())
	return sb.String()
}
