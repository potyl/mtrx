package mtrxgithub

import (
	"crypto/sha256"
	"os"
	"regexp"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// The attempt of a GitHub Actions workflow run, as seen from the environment of its jobs.
//
// All the spans of a run attempt share a trace id derived from the run, and the spans of each job
// hang from a span id derived from its runner name. The runner name is the only job identifier
// known both inside the job and by the jobs API, it's unique per job when the runners are ephemeral.
// This way the jobs and the export of the run (ExportRun) agree on the ids without passing anything
// between them.
type Run struct {
	Repository string // GITHUB_REPOSITORY: "org/repo"
	ID         string // GITHUB_RUN_ID
	Attempt    string // GITHUB_RUN_ATTEMPT
	Ref        string // GITHUB_REF: "refs/pull/123/merge" for a pull request
}

// Returns the run of the current GitHub Actions job, false when not running in GitHub Actions.
func RunFromEnv() (Run, bool) {
	run := Run{
		Repository: os.Getenv("GITHUB_REPOSITORY"),
		ID:         os.Getenv("GITHUB_RUN_ID"),
		Attempt:    os.Getenv("GITHUB_RUN_ATTEMPT"),
		Ref:        os.Getenv("GITHUB_REF"),
	}
	inActions := os.Getenv("GITHUB_ACTIONS") == "true" && run.Repository != "" && run.ID != "" && run.Attempt != ""
	return run, inActions
}

func (run Run) TraceID() (traceID trace.TraceID) {
	copy(traceID[:], run.hash("trace"))
	return traceID
}

func (run Run) RootSpanID() (spanID trace.SpanID) {
	copy(spanID[:], run.hash("root"))
	return spanID
}

func (run Run) JobSpanID(runnerName string) (spanID trace.SpanID) {
	copy(spanID[:], run.hash("job/"+runnerName))
	return spanID
}

// The span context of the job running on the given runner, to be used as the parent of the job's spans.
func (run Run) JobSpanContext(runnerName string) trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    run.TraceID(),
		SpanID:     run.JobSpanID(runnerName),
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
}

var pullRequestRef = regexp.MustCompile(`^refs/pull/(\d+)/`)

// The attributes identifying the run and, for a pull request, the change.
func (run Run) Attributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("cicd.pipeline.run.id", run.ID),
		attribute.String("github.run.attempt", run.Attempt),
	}
	if match := pullRequestRef.FindStringSubmatch(run.Ref); match != nil {
		attrs = append(attrs, attribute.String("vcs.change.id", match[1]))
	}
	return attrs
}

// The sha256 of the run attempt joined with the given suffix.
func (run Run) hash(suffix string) []byte {
	sum := sha256.Sum256([]byte(run.Repository + "/" + run.ID + "/" + run.Attempt + "/" + suffix))
	return sum[:]
}
