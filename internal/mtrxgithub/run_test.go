package mtrxgithub

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

var run = Run{ServerURL: "https://github.com", Repository: "cloudbeds/mfd", ID: "40561899477", Attempt: "1", Ref: "refs/pull/13140/merge"}

// The ids are the ones computed by: printf '%s' "$repo/$run_id/$attempt/$suffix" | sha256sum
func TestIDs(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"trace id", run.TraceID().String(), "59840d3e9264f39f5cbc97be40b9f257"},
		{"root span id", run.RootSpanID().String(), "6ff929a3d2adde82"},
		{"job span id", run.JobSpanID("x16-core-abc").String(), "0c719d7ef21030b0"},
	}
	for _, test := range tests {
		if test.got != test.expected {
			t.Errorf("%v: got %v, expected %v", test.name, test.got, test.expected)
		}
	}
}

func TestJobSpanContext(t *testing.T) {
	spanContext := run.JobSpanContext("x16-core-abc")
	if !spanContext.IsValid() || !spanContext.IsRemote() || !spanContext.IsSampled() {
		t.Errorf("expected a valid, remote and sampled span context, got %+v", spanContext)
	}
	if spanContext.SpanID() != run.JobSpanID("x16-core-abc") {
		t.Errorf("got span id %v, expected the job span id", spanContext.SpanID())
	}
}

func TestAttributes(t *testing.T) {
	tests := []struct {
		ref      string
		expected []attribute.KeyValue
	}{
		{
			"refs/pull/13140/merge",
			[]attribute.KeyValue{
				attribute.String("cicd.pipeline.run.id", "40561899477"),
				attribute.String("github.run.attempt", "1"),
				attribute.String("cicd.pipeline.run.url.full", "https://github.com/cloudbeds/mfd/actions/runs/40561899477/attempts/1"),
				attribute.String("vcs.change.id", "13140"),
			},
		},
		{
			"refs/heads/main",
			[]attribute.KeyValue{
				attribute.String("cicd.pipeline.run.id", "40561899477"),
				attribute.String("github.run.attempt", "1"),
				attribute.String("cicd.pipeline.run.url.full", "https://github.com/cloudbeds/mfd/actions/runs/40561899477/attempts/1"),
			},
		},
	}
	for _, test := range tests {
		run := run
		run.Ref = test.ref
		got := run.Attributes()
		if len(got) != len(test.expected) {
			t.Fatalf("%v: got %v, expected %v", test.ref, got, test.expected)
		}
		for i := range got {
			if got[i] != test.expected[i] {
				t.Errorf("%v: got %v, expected %v", test.ref, got[i], test.expected[i])
			}
		}
	}
}

func TestRunFromEnv(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", run.ServerURL)
	t.Setenv("GITHUB_REPOSITORY", run.Repository)
	t.Setenv("GITHUB_RUN_ID", run.ID)
	t.Setenv("GITHUB_RUN_ATTEMPT", run.Attempt)
	t.Setenv("GITHUB_REF", run.Ref)

	t.Setenv("GITHUB_ACTIONS", "")
	if _, ok := RunFromEnv(); ok {
		t.Error("expected no run outside of GitHub Actions")
	}

	t.Setenv("GITHUB_ACTIONS", "true")
	got, ok := RunFromEnv()
	if !ok || got != run {
		t.Errorf("got %+v (%v), expected %+v", got, ok, run)
	}
}
