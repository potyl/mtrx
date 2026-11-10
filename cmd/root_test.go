package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Runs mtrx with the given arguments and returns its output and error
func runRoot(t *testing.T, args ...string) (string, error) {
	// The telemetry of the commands run by the tests is printed instead of being sent
	t.Setenv("OTEL_EXPORTER", "console")

	var output bytes.Buffer
	root := newRootCmd("1.2.3")
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(args)
	err := root.Execute()
	return output.String(), err
}

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		output, err := runRoot(t, args...)
		if err != nil || output != "mtrx 1.2.3\n" {
			t.Errorf("%v: got %q and %v, expected %q", args, output, err, "mtrx 1.2.3\n")
		}
	}
}

func TestResolvedVersion(t *testing.T) {
	if got := resolvedVersion(""); got != "dev" {
		t.Errorf("got %q, expected dev without a version", got)
	}
	if got := resolvedVersion("2.0.0"); got != "2.0.0" {
		t.Errorf("got %q, expected 2.0.0", got)
	}
}

func TestExecPassesTheFlagsToTheCommand(t *testing.T) {
	// The command fails if mtrx parses its flags: sh exits with the number of arguments it got
	_, err := runRoot(t, "exec", "sh", "-c", "exit $#", "sh", "--quiet", "-v")

	var exit exitError
	if !errors.As(err, &exit) || exit.code != 2 {
		t.Errorf("got %v, expected the command to get its 2 flags and exit with 2", err)
	}
}

func TestExecReturnsTheExitCode(t *testing.T) {
	if _, err := runRoot(t, "exec", "true"); err != nil {
		t.Errorf("got %v, expected no error for a successful command", err)
	}

	_, err := runRoot(t, "exec", "--", "sh", "-c", "exit 3")
	var exit exitError
	if !errors.As(err, &exit) || exit.code != 3 {
		t.Errorf("got %v, expected the exit code 3", err)
	}
}

func TestExecName(t *testing.T) {
	// --name is mtrx's, the flags after the command stay the command's
	_, err := runRoot(t, "exec", "--name", "tests", "sh", "-c", "exit $#", "sh", "--name", "x")

	var exit exitError
	if !errors.As(err, &exit) || exit.code != 2 {
		t.Errorf("got %v, expected the command to get its 2 arguments and exit with 2", err)
	}
}

func TestParseAttributes(t *testing.T) {
	attrs, err := parseAttributes([]string{"test.suite.name=API", "test.suite.shard=2/4", "query=a=b", "empty="})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"test.suite.name=API", "test.suite.shard=2/4", "query=a=b", "empty="}
	for i, attr := range attrs {
		if got := string(attr.Key) + "=" + attr.Value.AsString(); got != expected[i] {
			t.Errorf("got %q, expected %q", got, expected[i])
		}
	}

	for _, invalid := range []string{"no-equal", "=value"} {
		if _, err := parseAttributes([]string{invalid}); err == nil {
			t.Errorf("%q: expected an error", invalid)
		}
	}
}

func TestExecRejectsAnInvalidAttribute(t *testing.T) {
	// The command must not run: it would exit with 3
	_, err := runRoot(t, "exec", "--attribute", "invalid", "sh", "-c", "exit 3")

	var exit exitError
	if err == nil || errors.As(err, &exit) {
		t.Errorf("got %v, expected an invalid attribute error without running the command", err)
	}
}

func TestExecRequiresACommand(t *testing.T) {
	if _, err := runRoot(t, "exec"); err == nil {
		t.Error("expected an error without a command")
	}
}

func TestBareCommandIsRejected(t *testing.T) {
	if _, err := runRoot(t, "true"); err == nil {
		t.Error("expected an error for a command without exec")
	}
}

// The job of the run of run_test.go, its ids are known
func setGitHubJobEnv(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY", "foo/bar")
	t.Setenv("GITHUB_RUN_ID", "40561899477")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("RUNNER_NAME", "x16-core-abc")
	t.Setenv("TRACEPARENT", "")
	t.Setenv("TRACESTATE", "")
	t.Setenv("BAGGAGE", "")
	t.Setenv("GITHUB_OUTPUT", "")
}

func TestGitHubContextOutsideOfGitHubActions(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("TRACEPARENT", "")
	if _, err := runRoot(t, "github", "context"); err == nil {
		t.Error("expected an error outside of GitHub Actions without TRACEPARENT")
	}
}

func TestGitHubContextOfTheJob(t *testing.T) {
	setGitHubJobEnv(t)
	outputPath := filepath.Join(t.TempDir(), "output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	output, err := runRoot(t, "github", "context")
	expected := "TRACEPARENT=00-f545254ee3b570264c97ca2bb99aae46-5e64f39631b1bf06-01\n"
	if err != nil || output != expected {
		t.Errorf("got %q and %v, expected %q", output, err, expected)
	}

	outputs, err := os.ReadFile(outputPath)
	expectedOutputs := "traceparent=00-f545254ee3b570264c97ca2bb99aae46-5e64f39631b1bf06-01\ntrace-id=f545254ee3b570264c97ca2bb99aae46\n"
	if err != nil || string(outputs) != expectedOutputs {
		t.Errorf("got %q and %v, expected the step outputs %q", outputs, err, expectedOutputs)
	}
}

func TestGitHubContextPassesTheEnvironmentThrough(t *testing.T) {
	setGitHubJobEnv(t)
	t.Setenv("TRACEPARENT", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	t.Setenv("TRACESTATE", "vendor=x")
	t.Setenv("BAGGAGE", "team=ci")

	output, err := runRoot(t, "github", "context")
	expected := "TRACEPARENT=00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01\nTRACESTATE=vendor=x\nBAGGAGE=team=ci\n"
	if err != nil || output != expected {
		t.Errorf("got %q and %v, expected %q", output, err, expected)
	}
}

func TestGitHubExportOutsideOfGitHubActions(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	if _, err := runRoot(t, "github", "export"); err == nil {
		t.Error("expected an error outside of GitHub Actions")
	}
}
