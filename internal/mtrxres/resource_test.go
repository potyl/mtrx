package mtrxres

import (
	"maps"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
)

func TestBaseRef(t *testing.T) {
	tests := []struct {
		event        string
		baseRef      string
		refName      string
		refType      string
		expected     string
		expectedType string
	}{
		{"push", "", "main", "branch", "main", "branch"},
		{"push", "", "v1.4.0", "tag", "v1.4.0", "tag"},
		{"pull_request", "main", "13140/merge", "branch", "main", "branch"},
		{"merge_group", "", "gh-readonly-queue/main/pr-13140-1cd33039dc2f8a7f1d6c2b07c0ab5e4c8fd1e3a2", "branch", "main", "branch"},
		{"merge_group", "", "gh-readonly-queue/release/v8.368.0/pr-13131-7b56d377763a1f0b6e1e0c3c2d8a9b4f5e6d7c8b", "branch", "release/v8.368.0", "branch"},
		{"workflow_dispatch", "", "emo/ci-mtrix-grafana", "branch", "emo/ci-mtrix-grafana", "branch"},
		{"local", "", "", "", "", ""},
	}
	for _, test := range tests {
		t.Setenv("GITHUB_BASE_REF", test.baseRef)
		t.Setenv("GITHUB_REF_NAME", test.refName)
		t.Setenv("GITHUB_REF_TYPE", test.refType)
		if got, gotType := baseRef(); got != test.expected || gotType != test.expectedType {
			t.Errorf("%v %q: got %q %q, expected %q %q", test.event, test.refName, got, gotType, test.expected, test.expectedType)
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
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "cloudbeds/mfd")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "cloudbeds")
	t.Setenv("GITHUB_SHA", "1cd33039dc2f8a7f1d6c2b07c0ab5e4c8fd1e3a2")
	t.Setenv("GITHUB_WORKFLOW", "PR Checks")
	t.Setenv("GITHUB_REF_TYPE", "branch")
	t.Setenv("GITHUB_REF_NAME", "13140/merge")
	t.Setenv("GITHUB_HEAD_REF", "emo/feature")
	t.Setenv("GITHUB_BASE_REF", "main")
	t.Setenv("GITHUB_ACTOR", "octocat")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")

	expected := map[attribute.Key]string{
		"vcs.provider.name":       "github",
		"vcs.owner.name":          "cloudbeds",
		"vcs.repository.name":     "mfd",
		"vcs.repository.url.full": "https://github.com/cloudbeds/mfd",
		"vcs.ref.head.name":       "emo/feature",
		"vcs.ref.head.type":       "branch",
		"vcs.ref.head.revision":   "1cd33039dc2f8a7f1d6c2b07c0ab5e4c8fd1e3a2",
		"vcs.ref.base.name":       "main",
		"vcs.ref.base.type":       "branch",
		"cicd.pipeline.name":      "PR Checks",
		"github.actor":            "octocat",
		"github.event.name":       "pull_request",
	}
	if got := attributesByKey(NewGitHubResource()); !maps.Equal(got, expected) {
		t.Errorf("got %v, expected %v", got, expected)
	}
}

func TestGitHubJobResource(t *testing.T) {
	tests := []struct {
		arch, os         string
		hostArch, osType string
	}{
		{"X64", "Linux", "amd64", "linux"},
		{"ARM64", "Linux", "arm64", "linux"},
		{"RISCV", "macOS", "riscv", "darwin"},
	}
	for _, test := range tests {
		t.Setenv("GITHUB_JOB", "phpstan")
		t.Setenv("RUNNER_NAME", "x16-core-abc")
		t.Setenv("RUNNER_ENVIRONMENT", "self-hosted")
		t.Setenv("RUNNER_ARCH", test.arch)
		t.Setenv("RUNNER_OS", test.os)

		expected := map[attribute.Key]string{
			"cicd.pipeline.task.name":   "phpstan",
			"cicd.worker.name":          "x16-core-abc",
			"github.runner.environment": "self-hosted",
			"host.arch":                 test.hostArch,
			"os.type":                   test.osType,
		}
		if got := attributesByKey(NewGitHubJobResource()); !maps.Equal(got, expected) {
			t.Errorf("%v %v: got %v, expected %v", test.arch, test.os, got, expected)
		}
	}
}
