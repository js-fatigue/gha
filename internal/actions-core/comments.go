package ac

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v68/github"
)

// UpsertPRComment deletes any existing comment whose body contains marker,
// then creates a new comment with body. No-ops silently if not on a PR.
func UpsertPRComment(ctx *Context, marker, body string) {
	goCtx := context.Background()
	repo, err := ctx.Repo()
	if err != nil {
		Warning(fmt.Sprintf("skipping PR comment: could not determine repo: %v", err), nil)
		return
	}
	client, err := NewClient()
	if err != nil {
		Warning(fmt.Sprintf("skipping PR comment: %v", err), nil)
		return
	}
	prNumber := resolvePRNumber(ctx, repo, client, goCtx)
	if prNumber == 0 {
		return
	}
	deletePRCommentByMarker(repo, client, goCtx, prNumber, marker)
	comment, _, err := client.Issues.CreateComment(goCtx, repo.Owner, repo.Repo, prNumber,
		&github.IssueComment{Body: &body})
	if err != nil {
		Warning(fmt.Sprintf("could not create PR comment: %v", err), nil)
		return
	}
	Info(fmt.Sprintf("Created PR comment #%d", comment.GetID()))
}

// DeletePRComment deletes any existing comment whose body contains marker.
// No-ops silently if not on a PR or no matching comment exists.
func DeletePRComment(ctx *Context, marker string) {
	goCtx := context.Background()
	repo, err := ctx.Repo()
	if err != nil {
		return
	}
	client, err := NewClient()
	if err != nil {
		return
	}
	prNumber := resolvePRNumber(ctx, repo, client, goCtx)
	if prNumber == 0 {
		return
	}
	deletePRCommentByMarker(repo, client, goCtx, prNumber, marker)
}

// resolvePRNumber returns the PR number from the webhook payload, falling back
// to querying the API for an open PR on the current branch. Returns 0 if none found.
func resolvePRNumber(ctx *Context, repo RepoInfo, client *github.Client, goCtx context.Context) int {
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

// deletePRCommentByMarker paginates PR comments and deletes the first one containing marker.
func deletePRCommentByMarker(repo RepoInfo, client *github.Client, goCtx context.Context, prNumber int, marker string) {
	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		comments, resp, err := client.Issues.ListComments(goCtx, repo.Owner, repo.Repo, prNumber, opts)
		if err != nil {
			Warning(fmt.Sprintf("could not list PR comments: %v", err), nil)
			return
		}
		for _, c := range comments {
			if strings.Contains(c.GetBody(), marker) {
				_, err = client.Issues.DeleteComment(goCtx, repo.Owner, repo.Repo, c.GetID())
				if err != nil {
					Warning(fmt.Sprintf("could not delete PR comment: %v", err), nil)
				} else {
					Info("Removed stale PR comment")
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
