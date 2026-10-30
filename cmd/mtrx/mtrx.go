package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mtrx/internal/mtrxgithub"
	"mtrx/internal/mtrxmeter"
	"mtrx/internal/mtrxotel"
	"mtrx/internal/mtrxproc"
	"mtrx/internal/mtrxres"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Exports the root and job spans of the current GitHub Actions run instead of running a command
const exportGitHubRunFlag = "--export-github-run"

// The release, set at build time with -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if len(os.Args) == 1 {
		name := filepath.Base(os.Args[0])
		fmt.Printf("Usage: %v CMD [ARG]...\n       %v %v\n", name, name, exportGitHubRunFlag)
		return
	}

	if os.Args[1] == exportGitHubRunFlag {
		os.Exit(exportGitHubRun())
	}

	// Command to start
	cmd := os.Args[1:]

	// Run the main logic
	exitCode := exec(cmd)

	// Exit with the same exist status as the child process
	os.Exit(exitCode)
}

func exec(cmd []string) int {
	// Create the OpenTelemetry objects
	ctx := context.Background()

	// The run, repository and job are the same for everything the process sends: they go on the
	// resource of the traces, the span only gets what describes the execution
	envResource, err := mtrxres.MergeResources(mtrxres.NewGitHubResource(), mtrxres.NewGitHubJobResource())
	if err != nil {
		log.Fatal(err)
	}
	traceAttrs := envResource.Attributes()

	run, inGitHubActions := mtrxgithub.RunFromEnv()
	if inGitHubActions {
		traceAttrs = append(traceAttrs, run.Attributes()...)
	}

	openTelemetry, err := mtrxotel.New(ctx, "mtrx", version, traceAttrs...)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		// A telemetry failure is reported but doesn't change the exit status of the command
		if err := openTelemetry.Shutdown(ctx); err != nil {
			log.Print(err)
		}
	}()

	// The span's parent is taken from the environment (TRACEPARENT) if present, otherwise in GitHub Actions
	// it's the span of the job, exported at the end of the run by --export-github-run
	parentCtx := mtrxotel.ContextFromEnv(ctx)
	if inGitHubActions && !trace.SpanContextFromContext(parentCtx).IsValid() {
		parentCtx = trace.ContextWithRemoteSpanContext(parentCtx, run.JobSpanContext(os.Getenv("RUNNER_NAME")))
	}

	// Start a span
	spanCtx, span := openTelemetry.StartSpan(parentCtx, strings.Join(cmd, " "))
	defer span.End()

	// Start the child process with our span as its parent
	process, err := mtrxproc.InstrumentProcess(cmd, mtrxotel.EnvironWithContext(spanCtx))
	if err != nil {
		span.SetStatus(codes.Error, "Child process execution and waiting encountered an error")
		span.RecordError(err)
		log.Fatal(err)
	}
	span.AddEvent("Child process finished")
	setExitStatus(span, process.Metrics.ExitCode)

	// The span describes the execution: the process and its resource usage
	processResource := mtrxres.NewProcessRuntimeResource(process)
	span.SetAttributes(processResource.Attributes()...)
	span.SetAttributes(mtrxmeter.UsageAttributes(process)...)

	// The metrics pick their low cardinality attributes among the environment and process ones
	resource, err := mtrxres.MergeResources(envResource, processResource)
	if err != nil {
		log.Fatal(err)
	}

	meter := openTelemetry.Meter()
	if err := mtrxmeter.CommandMetrics(ctx, meter, resource, process); err != nil {
		log.Fatal(err)
	}

	// Exit with the same exist status as the child process
	return process.Metrics.ExitCode
}

func exportGitHubRun() int {
	ctx := context.Background()

	run, inGitHubActions := mtrxgithub.RunFromEnv()
	if !inGitHubActions {
		log.Printf("%v must run in a GitHub Actions job", exportGitHubRunFlag)
		return 1
	}

	client, err := mtrxgithub.NewClientFromEnv()
	if err != nil {
		log.Print(err)
		return 1
	}

	// The spans are exported as they end and the SDK only reports the export errors to its handler
	var exportErr error
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		exportErr = errors.Join(exportErr, err)
	}))

	// Only the run attributes: the job ones would describe the job doing the export
	traceAttrs := append(mtrxres.NewGitHubResource().Attributes(), run.Attributes()...)
	openTelemetry, err := mtrxotel.New(ctx, "github-actions", version, traceAttrs...)
	if err != nil {
		log.Print(err)
		return 1
	}

	jobCount, err := mtrxgithub.ExportRun(ctx, openTelemetry, client, run)
	err = errors.Join(err, openTelemetry.Shutdown(ctx), exportErr)
	if err != nil {
		log.Print(err)
		return 1
	}

	fmt.Printf("Exported trace %v: run %v attempt %v with %v job spans\n", run.TraceID(), run.ID, run.Attempt, jobCount)
	return 0
}

// Records the exit code of the command on its span, a non zero exit code makes the span an error
func setExitStatus(span trace.Span, exitCode int) {
	span.SetAttributes(attribute.Int("process.exit.code", exitCode))
	if exitCode == 0 {
		span.SetStatus(codes.Ok, "")
		return
	}
	// The conventions use the exit code as error.type for a process when there's no better type
	span.SetAttributes(attribute.String("error.type", strconv.Itoa(exitCode)))
	span.SetStatus(codes.Error, fmt.Sprintf("Exit code %d", exitCode))
}
