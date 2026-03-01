package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-github/v68/github"

	ac "github.com/bshore/gha/internal/actions-core"
)

const prCommentMarker = "<!-- gha-example-context -->"

func main() {
	defer ac.Exit()

	ctx, err := ac.NewContext()
	if err != nil {
		ac.SetFailed(fmt.Sprintf("failed to read context: %v", err))
		return
	}

	repo, repoErr := ctx.Repo()

	logContext(ctx, repo, repoErr)
	writeJobSummary(ctx, repo, repoErr)
	upsertPRComment(ctx, repo)
}

func logContext(ctx *ac.Context, repo ac.RepoInfo, repoErr error) {
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

	if repoErr != nil {
		ac.Warning(fmt.Sprintf("could not determine repo: %v", repoErr), nil)
		return
	}

	ac.Group("Repository", func() error {
		ac.Info(fmt.Sprintf("Owner: %s", repo.Owner))
		ac.Info(fmt.Sprintf("Repo:  %s", repo.Repo))
		return nil
	})
}

func contextRows(ctx *ac.Context) [][]ac.SummaryTableCell {
	header := []ac.SummaryTableCell{
		{Data: "Field", Header: true},
		{Data: "Value", Header: true},
	}
	rows := [][]ac.SummaryTableCell{header}
	fields := [][2]string{
		{"Event", ctx.EventName},
		{"Workflow", ctx.Workflow},
		{"Action", ctx.Action},
		{"Actor", ctx.Actor},
		{"Job", ctx.Job},
		{"Run ID", fmt.Sprintf("%d", ctx.RunID)},
		{"Run Number", fmt.Sprintf("%d", ctx.RunNumber)},
		{"Run Attempt", fmt.Sprintf("%d", ctx.RunAttempt)},
		{"SHA", ctx.SHA},
		{"Ref", ctx.Ref},
		{"Server URL", ctx.ServerURL},
		{"API URL", ctx.APIURL},
		{"GraphQL URL", ctx.GraphQLURL},
	}
	for _, f := range fields {
		rows = append(rows, []ac.SummaryTableCell{{Data: f[0]}, {Data: f[1]}})
	}
	return rows
}

func writeJobSummary(ctx *ac.Context, repo ac.RepoInfo, repoErr error) {
	ac.JobSummary.
		AddHeading("Workflow Context", 2).
		AddTable(contextRows(ctx)).
		AddSeparator()

	if repoErr == nil {
		ac.JobSummary.
			AddHeading("Repository", 2).
			AddTable([][]ac.SummaryTableCell{
				{{Data: "Field", Header: true}, {Data: "Value", Header: true}},
				{{Data: "Owner"}, {Data: repo.Owner}},
				{{Data: "Repo"}, {Data: repo.Repo}},
			}).
			AddSeparator().
			AddHeading("Context Hash (SHA-256)", 2).
			AddCodeBlock(computeContextHash(ctx, repo), "")
	}

	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}
}

func computeContextHash(ctx *ac.Context, repo ac.RepoInfo) string {
	fields := []string{
		"action=" + ctx.Action,
		"actor=" + ctx.Actor,
		"apiURL=" + ctx.APIURL,
		"event=" + ctx.EventName,
		"graphqlURL=" + ctx.GraphQLURL,
		"job=" + ctx.Job,
		"ref=" + ctx.Ref,
		"repo=" + repo.Owner + "/" + repo.Repo,
		"runAttempt=" + fmt.Sprintf("%d", ctx.RunAttempt),
		"runID=" + fmt.Sprintf("%d", ctx.RunID),
		"runNumber=" + fmt.Sprintf("%d", ctx.RunNumber),
		"serverURL=" + ctx.ServerURL,
		"sha=" + ctx.SHA,
		"workflow=" + ctx.Workflow,
	}
	sort.Strings(fields)
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return fmt.Sprintf("%x", sum)
}

func buildCommentBody(ctx *ac.Context, repo ac.RepoInfo) string {
	var sb strings.Builder
	sb.WriteString(prCommentMarker + "\n")
	sb.WriteString("## Workflow Context\n\n")
	sb.WriteString("| Field | Value |\n|-------|-------|\n")
	fields := [][2]string{
		{"Event", ctx.EventName},
		{"Workflow", ctx.Workflow},
		{"Action", ctx.Action},
		{"Actor", ctx.Actor},
		{"Job", ctx.Job},
		{"Run ID", fmt.Sprintf("%d", ctx.RunID)},
		{"Run Number", fmt.Sprintf("%d", ctx.RunNumber)},
		{"Run Attempt", fmt.Sprintf("%d", ctx.RunAttempt)},
		{"SHA", ctx.SHA},
		{"Ref", ctx.Ref},
		{"Server URL", ctx.ServerURL},
		{"API URL", ctx.APIURL},
		{"GraphQL URL", ctx.GraphQLURL},
	}
	for _, f := range fields {
		sb.WriteString(fmt.Sprintf("| %s | %s |\n", f[0], f[1]))
	}
	sb.WriteString("\n## Repository\n\n")
	sb.WriteString(fmt.Sprintf("**Owner:** %s  \n**Repo:** %s\n", repo.Owner, repo.Repo))
	sb.WriteString("\n## Context Hash (SHA-256)\n\n")
	sb.WriteString(fmt.Sprintf("```\n%s\n```\n", computeContextHash(ctx, repo)))
	return sb.String()
}

func resolvePRNumber(ctx *ac.Context, repo ac.RepoInfo, client *github.Client, goCtx context.Context) int {
	issueInfo, err := ctx.Issue()
	if err == nil && issueInfo.Number > 0 {
		return issueInfo.Number
	}

	branch := strings.TrimPrefix(ctx.Ref, "refs/heads/")
	if branch == ctx.Ref {
		return 0
	}

	prs, _, err := client.PullRequests.List(goCtx, repo.Owner, repo.Repo, &github.PullRequestListOptions{
		Head:  repo.Owner + ":" + branch,
		State: "open",
	})
	if err != nil || len(prs) == 0 {
		return 0
	}
	return prs[0].GetNumber()
}

func upsertPRComment(ctx *ac.Context, repo ac.RepoInfo) {
	goCtx := context.Background()

	client, err := ac.NewClient()
	if err != nil {
		ac.Warning(fmt.Sprintf("skipping PR comment: %v", err), nil)
		return
	}

	prNumber := resolvePRNumber(ctx, repo, client, goCtx)
	if prNumber == 0 {
		return
	}

	body := buildCommentBody(ctx, repo)

	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
outer:
	for {
		comments, resp, err := client.Issues.ListComments(goCtx, repo.Owner, repo.Repo, prNumber, opts)
		if err != nil {
			ac.Warning(fmt.Sprintf("could not list PR comments: %v", err), nil)
			return
		}
		for _, c := range comments {
			if strings.Contains(c.GetBody(), prCommentMarker) {
				_, err = client.Issues.DeleteComment(goCtx, repo.Owner, repo.Repo, c.GetID())
				if err != nil {
					ac.Warning(fmt.Sprintf("could not delete PR comment: %v", err), nil)
					return
				}
				break outer
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	comment, _, err := client.Issues.CreateComment(goCtx, repo.Owner, repo.Repo, prNumber, &github.IssueComment{
		Body: github.String(body),
	})
	if err != nil {
		ac.Warning(fmt.Sprintf("could not create PR comment: %v", err), nil)
		return
	}
	ac.Info(fmt.Sprintf("Created PR comment #%d", comment.GetID()))
}
