package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bshore/gha/gha-github/cmd"
	ac "github.com/bshore/gha/internal/actions-core"
)

const prCommentMarker = "<!-- gha-github-error -->"

func main() {
	defer ac.Exit()

	if len(os.Args) < 2 {
		ac.SetFailed("usage: gha-github <command>")
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
}

func buildErrorCommentBody(command string, cmdErr error) string {
	var sb strings.Builder
	sb.WriteString(prCommentMarker + "\n")
	sb.WriteString(fmt.Sprintf("## GitHub Actions `%s` Failed\n\n", command))
	sb.WriteString(fmt.Sprintf("**Error:** %s\n", cmdErr.Error()))
	return sb.String()
}
