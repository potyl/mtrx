package mtrxgithub

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v92/github"
)

const runAttemptPath = "/repos/cloudbeds/mfd/actions/runs/40561899477/attempts/1"

func newTestClient(t *testing.T, handler http.HandlerFunc) *github.Client {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", server.URL)
	client, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestFetchRunAttemptFollowsPagination(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case r.URL.Path == runAttemptPath:
			fmt.Fprint(w, `{"name": "PR Checks", "run_started_at": "2026-10-27T21:15:00Z"}`)
		case r.URL.Path == runAttemptPath+"/jobs" && r.URL.Query().Get("page") == "":
			w.Header().Set("Link", fmt.Sprintf(`<http://%v%v/jobs?page=2>; rel="next", <http://%v%v/jobs?page=2>; rel="last"`, r.Host, runAttemptPath, r.Host, runAttemptPath))
			fmt.Fprint(w, `{"jobs": [{"name": "PHPStan", "runner_name": "x16-core-abc", "started_at": "2026-10-27T21:16:30Z", "completed_at": "2026-10-27T21:20:00Z", "conclusion": "success"}]}`)
		case r.URL.Path == runAttemptPath+"/jobs":
			fmt.Fprint(w, `{"jobs": [{"name": "CI Trace", "runner_name": null, "started_at": "2026-10-27T21:30:00Z", "completed_at": null, "conclusion": null}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	attempt, jobs, err := fetchRunAttempt(context.Background(), client, run)
	if err != nil {
		t.Fatal(err)
	}

	if attempt.GetName() != "PR Checks" || attempt.GetRunStartedAt().IsZero() {
		t.Errorf("unexpected run attempt %+v", attempt)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %v jobs, expected 2: %+v", len(jobs), jobs)
	}
	if jobs[0].GetRunnerName() != "x16-core-abc" || jobs[0].CompletedAt == nil || jobs[0].GetConclusion() != "success" {
		t.Errorf("unexpected first job %+v", jobs[0])
	}
	if jobs[1].GetRunnerName() != "" || jobs[1].CompletedAt != nil {
		t.Errorf("unexpected second job %+v", jobs[1])
	}
}

func TestFetchRunAttemptFailsOnErrorStatus(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if _, _, err := fetchRunAttempt(context.Background(), client, run); err == nil {
		t.Error("expected an error on a 403 response")
	}
}

func TestFetchRunAttemptRejectsInvalidRun(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %v", r.URL)
	})

	for _, invalid := range []Run{
		{Repository: "mfd", ID: run.ID, Attempt: run.Attempt},
		{Repository: run.Repository, ID: "abc", Attempt: run.Attempt},
		{Repository: run.Repository, ID: run.ID, Attempt: "first"},
	} {
		if _, _, err := fetchRunAttempt(context.Background(), client, invalid); err == nil {
			t.Errorf("expected an error for %+v", invalid)
		}
	}
}

func TestNewClientFromEnvRequiresToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	if _, err := NewClientFromEnv(); err == nil {
		t.Error("expected an error without GITHUB_TOKEN")
	}
}
