package mtrxgithub

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

var exported = ExportedTrace{
	TraceID:  run.TraceID(),
	Start:    time.UnixMilli(1793350000123),
	End:      time.UnixMilli(1793351200456),
	JobCount: 35,
}

func TestOutputs(t *testing.T) {
	expected := []string{
		"trace-id=59840d3e9264f39f5cbc97be40b9f257",
		"start-time-ms=1793350000123",
		"end-time-ms=1793351200456",
		"job-count=35",
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
