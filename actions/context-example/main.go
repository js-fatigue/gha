package main

import (
	"fmt"

	ac "github.com/bshore/gha/internal/actions-core"
)

func main() {
	defer ac.Exit()

	ctx, err := ac.NewContext()
	if err != nil {
		ac.SetFailed(fmt.Sprintf("failed to read context: %v", err))
		return
	}

	ac.Group("Workflow Context", func() error {
		ac.Info(fmt.Sprintf("Event:       %s", ctx.EventName))
		ac.Info(fmt.Sprintf("Workflow:    %s", ctx.Workflow))
		ac.Info(fmt.Sprintf("Action:      %s", ctx.Action))
		ac.Info(fmt.Sprintf("Actor:       %s", ctx.Actor))
		ac.Info(fmt.Sprintf("Job:         %s", ctx.Job))
		ac.Info(fmt.Sprintf("Run ID:      %d", ctx.RunID))
		ac.Info(fmt.Sprintf("Run Number:  %d", ctx.RunNumber))
		ac.Info(fmt.Sprintf("Run Attempt: %d", ctx.RunAttempt))
		ac.Info(fmt.Sprintf("SHA:         %s", ctx.SHA))
		ac.Info(fmt.Sprintf("Ref:         %s", ctx.Ref))
		ac.Info(fmt.Sprintf("Server URL:  %s", ctx.ServerURL))
		ac.Info(fmt.Sprintf("API URL:     %s", ctx.APIURL))
		ac.Info(fmt.Sprintf("GraphQL URL: %s", ctx.GraphQLURL))
		return nil
	})

	repo, err := ctx.Repo()
	if err != nil {
		ac.Warning(fmt.Sprintf("could not determine repo: %v", err), nil)
		return
	}

	ac.Group("Repository", func() error {
		ac.Info(fmt.Sprintf("Owner: %s", repo.Owner))
		ac.Info(fmt.Sprintf("Repo:  %s", repo.Repo))
		return nil
	})
}
