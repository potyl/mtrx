package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// The exit status of a command run by mtrx, which mtrx exits with without reporting an error
type exitError struct {
	code int
}

func (err exitError) Error() string {
	return fmt.Sprintf("exit status %d", err.code)
}

// Execute is the entry point called from main.
func Execute(ver string) {
	if err := newRootCmd(resolvedVersion(ver)).Execute(); err != nil {
		var exit exitError
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// Returns "dev" when the version isn't set at build time.
func resolvedVersion(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}

func newRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mtrx",
		Short: "Measure commands and send their telemetry to OpenTelemetry",
		Long: `mtrx runs a command, like the time command, and sends its trace and metrics (duration, CPU
time, memory, page faults, context switches and file system I/O) to an OpenTelemetry collector.
In GitHub Actions the telemetry is annotated with the run, repository and job, and the commands
of a run are linked into a single trace.`,
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetVersionTemplate("mtrx {{.Version}}\n")

	cmd.AddCommand(
		newExecCmd(version),
		newGitHubCmd(version),
		newVersionCmd(version),
	)
	return cmd
}

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of mtrx",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "mtrx %s\n", version)
		},
	}
}
