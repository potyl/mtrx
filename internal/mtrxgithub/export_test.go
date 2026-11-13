package mtrxgithub

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Starts the spans with a tracer, like OpenTelemetry does
type tracerSpans struct{ tracer trace.Tracer }

func (spans tracerSpans) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return spans.tracer.Start(ctx, name, opts...)
}

func timestamp(value string) *github.Timestamp {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &github.Timestamp{Time: parsed}
}

func step(number int64, name string, conclusion string, start string, end string) *github.TaskStep {
	step := &github.TaskStep{Number: &number, Name: &name, Conclusion: &conclusion, StartedAt: timestamp(start)}
	if end != "" {
		step.CompletedAt = timestamp(end)
	}
	return step
}

func TestExportSteps(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter)).Tracer("")
	jobCtx, jobSpan := tracer.Start(context.Background(), "job")

	job := &github.WorkflowJob{Steps: []*github.TaskStep{
		step(1, "Checkout", "success", "2026-11-13T10:00:00Z", "2026-11-13T10:00:05Z"),
		step(2, "Tests", "failure", "2026-11-13T10:00:05Z", "2026-11-13T10:02:00Z"),
		step(3, "Deploy", "skipped", "2026-11-13T10:02:00Z", "2026-11-13T10:02:00Z"),
		step(4, "Complete job", "", "2026-11-13T10:02:00Z", ""),
	}}
	count := exportSteps(jobCtx, tracerSpans{tracer}, job)
	jobSpan.End()

	if count != 2 {
		t.Errorf("got %v steps, expected 2: the skipped step and the one that didn't end are left out", count)
	}
	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("got %v spans, expected the 2 steps and the job", len(spans))
	}

	expected := []struct {
		name   string
		status codes.Code
		start  string
		end    string
	}{
		{"Checkout", codes.Ok, "2026-11-13T10:00:00Z", "2026-11-13T10:00:05Z"},
		{"Tests", codes.Error, "2026-11-13T10:00:05Z", "2026-11-13T10:02:00Z"},
	}
	for i, want := range expected {
		got := spans[i]
		if got.Name != want.name || got.Status.Code != want.status {
			t.Errorf("span %v: got %v with status %v, expected %v with status %v", i, got.Name, got.Status.Code, want.name, want.status)
		}
		if !got.StartTime.Equal(timestamp(want.start).Time) || !got.EndTime.Equal(timestamp(want.end).Time) {
			t.Errorf("%v: got %v to %v, expected the step's %v to %v", got.Name, got.StartTime, got.EndTime, want.start, want.end)
		}
		if got.Parent.SpanID() != jobSpan.SpanContext().SpanID() {
			t.Errorf("%v: got parent %v, expected the job span %v", got.Name, got.Parent.SpanID(), jobSpan.SpanContext().SpanID())
		}
		if got.SpanContext.SpanID() == jobSpan.SpanContext().SpanID() {
			t.Errorf("%v: got the job's span id, expected its own", got.Name)
		}
	}
}

var exported = ExportedTrace{
	TraceID:   run.TraceID(),
	Start:     time.UnixMilli(1793350000123),
	End:       time.UnixMilli(1793351200456),
	JobCount:  35,
	StepCount: 240,
}

func TestOutputs(t *testing.T) {
	expected := []string{
		"trace-id=f545254ee3b570264c97ca2bb99aae46",
		"start-time-ms=1793350000123",
		"end-time-ms=1793351200456",
		"job-count=35",
		"step-count=240",
	}
	if got := exported.Outputs(); !slices.Equal(got, expected) {
		t.Errorf("got %v, expected %v", got, expected)
	}
}

func TestAppendOutputsKeepsTheExistingOutputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github_output")
	if err := os.WriteFile(path, []byte("previous=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AppendOutputs(path, []string{"trace-id=abc", "job-count=2"}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if expected := "previous=1\ntrace-id=abc\njob-count=2\n"; string(content) != expected {
		t.Errorf("got %q, expected %q", content, expected)
	}
}
