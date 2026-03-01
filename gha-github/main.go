package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v68/github"

	ac "github.com/bshore/gha/internal/actions-core"
	"github.com/bshore/gha/gha-github/cmd"
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
		postErrorPRComment(ctx, command, err)
	} else {
		deleteErrorPRComment(ctx)
	}
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

func buildErrorCommentBody(command string, cmdErr error) string {
	var sb strings.Builder
	sb.WriteString(prCommentMarker + "\n")
	sb.WriteString(fmt.Sprintf("## GitHub Actions `%s` Failed\n\n", command))
	sb.WriteString(fmt.Sprintf("**Error:** %s\n", cmdErr.Error()))
	return sb.String()
}

func postErrorPRComment(ctx *ac.Context, command string, cmdErr error) {
	goCtx := context.Background()

	repo, err := ctx.Repo()
	if err != nil {
		ac.Warning(fmt.Sprintf("skipping PR comment: could not determine repo: %v", err), nil)
		return
	}

	client, err := ac.NewClient()
	if err != nil {
		ac.Warning(fmt.Sprintf("skipping PR comment: %v", err), nil)
		return
	}

	prNumber := resolvePRNumber(ctx, repo, client, goCtx)
	if prNumber == 0 {
		return
	}

	body := buildErrorCommentBody(command, cmdErr)

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
	ac.Info(fmt.Sprintf("Created error PR comment #%d", comment.GetID()))
}

func deleteErrorPRComment(ctx *ac.Context) {
	goCtx := context.Background()

	repo, err := ctx.Repo()
	if err != nil {
		return
	}

	client, err := ac.NewClient()
	if err != nil {
		return
	}

	prNumber := resolvePRNumber(ctx, repo, client, goCtx)
	if prNumber == 0 {
		return
	}

	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
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
					ac.Warning(fmt.Sprintf("could not delete stale PR comment: %v", err), nil)
				} else {
					ac.Info("Removed stale error PR comment")
				}
				return
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
}
