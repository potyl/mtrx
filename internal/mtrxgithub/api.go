package mtrxgithub

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v92/github"
)

// Creates a GitHub client authenticated with GITHUB_TOKEN against GITHUB_API_URL (https://api.github.com by default).
func NewClientFromEnv() (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, errors.New("GITHUB_TOKEN is not set")
	}

	opts := []github.ClientOptionsFunc{
		github.WithAuthToken(token),
		github.WithTimeout(30 * time.Second),
	}
	if apiURL := os.Getenv("GITHUB_API_URL"); apiURL != "" {
		opts = append(opts, github.WithURLs(&apiURL, nil))
	}

	return github.NewClient(opts...)
}

// Returns the run attempt and all its jobs.
func fetchRunAttempt(ctx context.Context, client *github.Client, run Run) (*github.WorkflowRun, []*github.WorkflowJob, error) {
	owner, repo, found := strings.Cut(run.Repository, "/")
	if !found {
		return nil, nil, fmt.Errorf("invalid repository %q, expected owner/repo", run.Repository)
	}

	runID, err := strconv.ParseInt(run.ID, 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid run id %q: %w", run.ID, err)
	}

	attempt, err := strconv.Atoi(run.Attempt)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid run attempt %q: %w", run.Attempt, err)
	}

	workflowRun, _, err := client.Actions.GetWorkflowRunAttempt(ctx, owner, repo, runID, attempt, nil)
	if err != nil {
		return nil, nil, err
	}

	jobs := []*github.WorkflowJob{}
	for job, err := range client.Actions.ListWorkflowJobsAttemptIter(ctx, owner, repo, runID, int64(attempt), &github.ListOptions{PerPage: 100}) {
		if err != nil {
			return nil, nil, err
		}
		jobs = append(jobs, job)
	}

	return workflowRun, jobs, nil
}
