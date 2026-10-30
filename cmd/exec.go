package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"mtrx/internal/mtrxgithub"
	"mtrx/internal/mtrxmeter"
	"mtrx/internal/mtrxotel"
	"mtrx/internal/mtrxproc"
	"mtrx/internal/mtrxres"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func newExecCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec CMD [ARG]...",
		Short: "Run a command and send its trace and metrics",
		Long: `exec runs the command and exits with its exit status. Once it exits, its span and its metrics
are sent to the OpenTelemetry collector configured with the OTEL_* environment variables.

The flags after the command are the command's: mtrx exec docker compose build --pull.`,
		Example: "  mtrx exec docker compose build --with-dependencies cli",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if exitCode := execCommand(version, args); exitCode != 0 {
				return exitError{exitCode}
			}
			return nil
		},
	}
	// Everything from the command on is passed to it, flags included
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func execCommand(version string, cmd []string) int {
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
	// it's the span of the job, exported at the end of the run by mtrx github export
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
