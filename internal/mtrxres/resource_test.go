package mtrxres

import (
	"maps"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
)

func TestBaseRefName(t *testing.T) {
	tests := []struct {
		event    string
		baseRef  string
		refName  string
		expected string
	}{
		{"push", "", "main", "main"},
		{"pull_request", "main", "13140/merge", "main"},
		{"merge_group", "", "gh-readonly-queue/main/pr-13140-1cd33039dc2f8a7f1d6c2b07c0ab5e4c8fd1e3a2", "main"},
		{"merge_group", "", "gh-readonly-queue/release/v8.368.0/pr-13131-7b56d377763a1f0b6e1e0c3c2d8a9b4f5e6d7c8b", "release/v8.368.0"},
		{"workflow_dispatch", "", "emo/ci-mtrix-grafana", "emo/ci-mtrix-grafana"},
		{"local", "", "", ""},
	}
	for _, test := range tests {
		t.Setenv("GITHUB_BASE_REF", test.baseRef)
		t.Setenv("GITHUB_REF_NAME", test.refName)
		if got := baseRefName(); got != test.expected {
			t.Errorf("%v %q: got %q, expected %q", test.event, test.refName, got, test.expected)
		}
	}
}

// Returns the attributes of the resource by key
func attributesByKey(resource *sdkres.Resource) map[attribute.Key]string {
	attrs := map[attribute.Key]string{}
	for _, attr := range resource.Attributes() {
		attrs[attr.Key] = attr.Value.Emit()
	}
	return attrs
}

func TestGitHubResource(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "cloudbeds/mfd")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "cloudbeds")
	t.Setenv("GITHUB_REF_TYPE", "branch")
	t.Setenv("GITHUB_REF_NAME", "13140/merge")
	t.Setenv("GITHUB_HEAD_REF", "emo/feature")
	t.Setenv("GITHUB_BASE_REF", "main")
	t.Setenv("GITHUB_JOB", "phpstan")
	t.Setenv("GITHUB_ACTOR", "octocat")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")

	expected := map[attribute.Key]string{
		"vcs.owner.name":          "cloudbeds",
		"vcs.repository.name":     "mfd",
		"vcs.ref.head.name":       "emo/feature",
		"vcs.ref.head.type":       "branch",
		"vcs.ref.base.name":       "main",
		"cicd.pipeline.task.name": "phpstan",
		"github.actor":            "octocat",
		"github.event.name":       "pull_request",
	}
	if got := attributesByKey(NewGitHubResource()); !maps.Equal(got, expected) {
		t.Errorf("got %v, expected %v", got, expected)
	}
}

func TestGitHubRunnerResource(t *testing.T) {
	tests := []struct {
		arch, os         string
		hostArch, osType string
	}{
		{"X64", "Linux", "amd64", "linux"},
		{"ARM64", "Linux", "arm64", "linux"},
		{"RISCV", "macOS", "riscv", "darwin"},
	}
	for _, test := range tests {
		t.Setenv("RUNNER_NAME", "x16-core-abc")
		t.Setenv("RUNNER_ENVIRONMENT", "self-hosted")
		t.Setenv("RUNNER_ARCH", test.arch)
		t.Setenv("RUNNER_OS", test.os)

		expected := map[attribute.Key]string{
			"cicd.worker.name":          "x16-core-abc",
			"github.runner.environment": "self-hosted",
			"host.arch":                 test.hostArch,
			"os.type":                   test.osType,
		}
		if got := attributesByKey(NewGitHubRunnerResource()); !maps.Equal(got, expected) {
			t.Errorf("%v %v: got %v, expected %v", test.arch, test.os, got, expected)
		}
	}
}
