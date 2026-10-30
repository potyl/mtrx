package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"mtrx/internal/mtrxgithub"
	"mtrx/internal/mtrxotel"
	"mtrx/internal/mtrxres"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
)

func newGitHubCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "github",
		Short: "GitHub Actions commands",
	}
	cmd.AddCommand(newGitHubExportCmd(version))
	return cmd
}

func newGitHubExportCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export the trace of the current GitHub Actions run",
		Long: `export reads the current run attempt and its jobs from the GitHub API (GITHUB_TOKEN) and exports
a root span for the run and a span per job, the parents of the spans of the commands run with
mtrx exec. Run it from the last job of the workflow: every other job must have completed.

The step outputs (GITHUB_OUTPUT) get the trace-id, the start-time-ms and end-time-ms of the root
span in Unix milliseconds, and the job-count, for instance to link to the trace.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exportGitHubRun(cmd, version)
		},
	}
}

func exportGitHubRun(cmd *cobra.Command, version string) error {
	ctx := context.Background()

	run, inGitHubActions := mtrxgithub.RunFromEnv()
	if !inGitHubActions {
		return errors.New("mtrx github export must run in a GitHub Actions job")
	}

	client, err := mtrxgithub.NewClientFromEnv()
	if err != nil {
		return err
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
		return err
	}

	exported, err := mtrxgithub.ExportRun(ctx, openTelemetry, client, run)
	if err := errors.Join(err, openTelemetry.Shutdown(ctx), exportErr); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Exported trace %v: run %v attempt %v with %v job spans\n", exported.TraceID, run.ID, run.Attempt, exported.JobCount)

	if outputPath := os.Getenv("GITHUB_OUTPUT"); outputPath != "" {
		return mtrxgithub.AppendOutputs(outputPath, exported.Outputs())
	}
	return nil
}
