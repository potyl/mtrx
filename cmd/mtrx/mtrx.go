package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"mtrx/internal/mtrxmeter"
	"mtrx/internal/mtrxotel"
	"mtrx/internal/mtrxproc"
	"mtrx/internal/mtrxres"

	"go.opentelemetry.io/otel/codes"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Printf("Usage: %v CMD [ARG]...\n", filepath.Base(os.Args[0]))
		return
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

	openTelemetry, err := mtrxotel.New(ctx, "mtrx")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err := openTelemetry.Shutdown(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}()

	// Start a span
	_, span := openTelemetry.StartSpan(ctx, strings.Join(cmd, " "))
	defer span.End()

	// Start the child process
	process, err := mtrxproc.InstrumentProcess(cmd)
	if err != nil {
		span.SetStatus(codes.Error, "Child process execution and waiting encountered an error")
		span.RecordError(err)
		log.Fatal(err)
	}
	span.AddEvent("Child process finished")
	span.SetStatus(codes.Ok, "Child process finished")

	// Add the resources collected from environment and the child process
	resourcesList := []*sdkres.Resource{
		mtrxres.NewGitHubResource(),
		mtrxres.NewGitHubRunnerResource(),
		mtrxres.NewProcessRuntimeResource(process),
	}

	// Merge all resources into a single one
	resource, err := mtrxres.MergeResources(resourcesList...)
	if err != nil {
		log.Fatal(err)
	}

	// Enhance the span with attributes based on process
	span.SetAttributes(resource.Attributes()...)

	// TODO add
	// - https://opentelemetry.io/docs/specs/semconv/resource/host/
	// - https://opentelemetry.io/docs/specs/semconv/resource/os/

	meter := openTelemetry.Meter()
	if err := mtrxmeter.OsProcessMetrics(ctx, meter, resource, process); err != nil {
		log.Fatal(err)
	}

	// Exit with the same exist status as the child process
	return process.Metrics.ExitCode
}
