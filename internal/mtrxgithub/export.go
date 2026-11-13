package mtrxgithub

import (
	"context"
	"fmt"
	"os"
	"time"

	"mtrx/internal/mtrxotel"

	"github.com/google/go-github/v92/github"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// The trace of a run attempt exported by ExportRun
type ExportedTrace struct {
	TraceID   trace.TraceID
	Start     time.Time // The start of the run attempt, the start of the root span
	End       time.Time // The end of the root span: when the run was exported
	JobCount  int
	StepCount int
}

// Starts spans, implemented by mtrxotel.OpenTelemetry
type spanStarter interface {
	StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
}

// Exports the root span of the run attempt and one span per job, the parents of the spans that mtrx
// recorded in the jobs, and with steps a span per step of the jobs. It's meant to run from the last
// job of the workflow: every other job must have completed, and the run itself ends now.
func ExportRun(ctx context.Context, openTelemetry *mtrxotel.OpenTelemetry, client *github.Client, run Run, steps bool) (ExportedTrace, error) {
	attempt, jobs, err := fetchRunAttempt(ctx, client, run)
	if err != nil {
		return ExportedTrace{}, err
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
	stepCount := 0
	for _, job := range jobs {
		// Skipped jobs never got a runner and the jobs still running (this one) have no end
		if job.GetRunnerName() == "" || job.CompletedAt == nil {
			continue
		}

		jobCtx, span := openTelemetry.StartSpan(
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
		if steps {
			stepCount += exportSteps(jobCtx, openTelemetry, job)
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
	exported := ExportedTrace{
		TraceID:   run.TraceID(),
		Start:     attempt.GetRunStartedAt().Time,
		End:       time.Now(),
		JobCount:  jobCount,
		StepCount: stepCount,
	}
	_, span := openTelemetry.StartSpan(
		mtrxotel.ContextWithSpanID(rootCtx, run.RootSpanID()),
		attempt.GetName(),
		trace.WithTimestamp(exported.Start),
		trace.WithAttributes(attribute.String("cicd.pipeline.result", runConclusion)),
	)
	setStatus(span, runConclusion)
	span.End(trace.WithTimestamp(exported.End))

	return exported, nil
}

// Exports a span per step of the job as children of the job span of ctx and returns their count.
// The skipped steps and the ones that didn't end have no interval to show. The steps' failures
// don't count against the run: the job's conclusion already accounts for them.
func exportSteps(ctx context.Context, spans spanStarter, job *github.WorkflowJob) int {
	// The job's id is forced on ctx, the steps get random ids: nothing refers to them
	ctx = mtrxotel.ContextWithoutSpanID(ctx)

	count := 0
	for _, step := range job.Steps {
		if step.GetConclusion() == "skipped" || step.StartedAt == nil || step.CompletedAt == nil {
			continue
		}
		_, span := spans.StartSpan(
			ctx,
			step.GetName(),
			trace.WithTimestamp(step.GetStartedAt().Time),
			trace.WithAttributes(
				attribute.Int64("github.step.number", step.GetNumber()),
				attribute.String("github.step.result", step.GetConclusion()),
			),
		)
		setStatus(span, step.GetConclusion())
		span.End(trace.WithTimestamp(step.GetCompletedAt().Time))
		count++
	}
	return count
}

// The step outputs of the exported trace, the times are in Unix milliseconds as Grafana takes them.
func (exported ExportedTrace) Outputs() []string {
	return []string{
		"trace-id=" + exported.TraceID.String(),
		fmt.Sprintf("start-time-ms=%d", exported.Start.UnixMilli()),
		fmt.Sprintf("end-time-ms=%d", exported.End.UnixMilli()),
		fmt.Sprintf("job-count=%d", exported.JobCount),
		fmt.Sprintf("step-count=%d", exported.StepCount),
	}
}

// Appends the key=value outputs to the step outputs file of GitHub Actions (GITHUB_OUTPUT).
func AppendOutputs(path string, outputs []string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		if _, err := fmt.Fprintln(file, output); err != nil {
			file.Close()
			return err
		}
	}
	return file.Close()
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
