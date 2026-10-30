package cmd

import (
	"bytes"
	"errors"
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

func TestGitHubExportOutsideOfGitHubActions(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	if _, err := runRoot(t, "github", "export"); err == nil {
		t.Error("expected an error outside of GitHub Actions")
	}
}
