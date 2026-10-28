package mtrxgithub

import (
	"context"
	"fmt"
	"time"

	"mtrx/internal/mtrxotel"

	"github.com/google/go-github/v92/github"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Exports the root span of the run attempt and one span per job, the parents of the spans that mtrx
// recorded in the jobs. It's meant to run from the last job of the workflow: every other job must
// have completed, and the run itself ends now.
// Returns the number of job spans exported.
func ExportRun(ctx context.Context, openTelemetry *mtrxotel.OpenTelemetry, client *github.Client, run Run) (int, error) {
	attempt, jobs, err := fetchRunAttempt(ctx, client, run)
	if err != nil {
		return 0, err
	}

	// The jobs hang from the root span, which gets exported last as it ends now
	rootSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    run.TraceID(),
		SpanID:     run.RootSpanID(),
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	jobsCtx := trace.ContextWithRemoteSpanContext(ctx, rootSpanContext)

	runSucceeded := true
	jobCount := 0
	for _, job := range jobs {
		// Skipped jobs never got a runner and the jobs still running (this one) have no end
		if job.GetRunnerName() == "" || job.CompletedAt == nil {
			continue
		}

		_, span := openTelemetry.StartSpan(
			mtrxotel.ContextWithSpanID(jobsCtx, run.JobSpanID(job.GetRunnerName())),
			job.GetName(),
			trace.WithTimestamp(job.GetStartedAt().Time),
			trace.WithAttributes(
				attribute.String("cicd.pipeline.task.run.result", job.GetConclusion()),
				attribute.String("cicd.worker.name", job.GetRunnerName()),
			),
		)
		if !setStatus(span, job.GetConclusion()) {
			runSucceeded = false
		}
		span.End(trace.WithTimestamp(job.GetCompletedAt().Time))
		jobCount++
	}

	// A span context with a trace id and no span id gives the root span its trace id but no parent
	rootCtx := trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{TraceID: run.TraceID()}))
	runConclusion := "failure"
	if runSucceeded {
		runConclusion = "success"
	}
	_, span := openTelemetry.StartSpan(
		mtrxotel.ContextWithSpanID(rootCtx, run.RootSpanID()),
		attempt.GetName(),
		trace.WithTimestamp(attempt.GetRunStartedAt().Time),
		trace.WithAttributes(attribute.String("cicd.pipeline.result", runConclusion)),
	)
	setStatus(span, runConclusion)
	span.End(trace.WithTimestamp(time.Now()))

	return jobCount, nil
}

// Sets the span status for the given conclusion and returns whether it's a successful one.
func setStatus(span trace.Span, conclusion string) bool {
	switch conclusion {
	case "success", "skipped", "neutral":
		span.SetStatus(codes.Ok, "")
		return true
	default:
		span.SetStatus(codes.Error, fmt.Sprintf("Conclusion: %s", conclusion))
		return false
	}
}
