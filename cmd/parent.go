package cmd

import (
	"context"
	"os"

	"mtrx/internal/mtrxgithub"
	"mtrx/internal/mtrxotel"

	"go.opentelemetry.io/otel/trace"
)

// The parent of the spans of a job: the environment (TRACEPARENT) when set, otherwise in GitHub
// Actions the span of the job, exported at the end of the run by mtrx github export.
func parentContext(ctx context.Context, run mtrxgithub.Run, inGitHubActions bool) context.Context {
	parentCtx := mtrxotel.ContextFromEnv(ctx)
	if inGitHubActions && !trace.SpanContextFromContext(parentCtx).IsValid() {
		parentCtx = trace.ContextWithRemoteSpanContext(parentCtx, run.JobSpanContext(os.Getenv("RUNNER_NAME")))
	}
	return parentCtx
}
