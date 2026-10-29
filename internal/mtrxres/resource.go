package mtrxres

import (
	"mtrx/internal/mtrxtypes"
	"os"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

//
// Process Runtime Attributes
//

func NewProcessRuntimeResource(process *mtrxtypes.Process) *sdkres.Resource {
	attrs := []attribute.KeyValue{
		semconv.ProcessCommand(process.Command),
		semconv.ProcessCommandArgs(process.CommandArgs...),
		semconv.ProcessExecutableName(process.Executable.Name),
		semconv.ProcessExecutablePath(process.Executable.Path),
		semconv.ProcessOwner(process.Owner),
		semconv.ProcessParentPID(process.ParentPID),
		semconv.ProcessPID(process.PID),
	}
	return sdkres.NewSchemaless(attrs...)
}

//
// GitHub Attributes
//

// The attributes of the repository, refs and job, named after the OpenTelemetry semantic conventions
// (vcs.*, cicd.*) when they define them, under github.* otherwise.
func NewGitHubResource() *sdkres.Resource {
	attrs := []attribute.KeyValue{
		envKeyValue("GITHUB_REPOSITORY_OWNER", "vcs.owner.name"), // "cloudbeds"
		envKeyValue("GITHUB_REF_TYPE", "vcs.ref.head.type"),      // "branch" or "tag"
		envKeyValue("GITHUB_SHA", "vcs.ref.head.revision"),       // the merge commit for a pull request
		envKeyValue("GITHUB_WORKFLOW", "cicd.pipeline.name"),     // "PR Checks"
		envKeyValue("GITHUB_JOB", "cicd.pipeline.task.name"),     // "test"
		envKeyValue("GITHUB_ACTOR", "github.actor"),              // "username"
		envKeyValue("GITHUB_EVENT_NAME", "github.event.name"),    // "pull_request"
	}
	// The repository name without its owner: "mfd" for "cloudbeds/mfd"
	repository := os.Getenv("GITHUB_REPOSITORY")
	if _, name, found := strings.Cut(repository, "/"); found {
		attrs = append(
			attrs,
			attribute.String("vcs.provider.name", "github"),
			attribute.String("vcs.repository.name", name),
		)
		if serverURL := os.Getenv("GITHUB_SERVER_URL"); serverURL != "" {
			attrs = append(attrs, attribute.String("vcs.repository.url.full", serverURL+"/"+repository))
		}
	}
	// The branch of a pull request, GITHUB_REF_NAME is "<number>/merge" for it
	if headRefName := firstEnv("GITHUB_HEAD_REF", "GITHUB_REF_NAME"); headRefName != "" {
		attrs = append(attrs, attribute.String("vcs.ref.head.name", headRefName))
	}
	if baseRefName, baseRefType := baseRef(); baseRefName != "" {
		attrs = append(
			attrs,
			attribute.String("vcs.ref.base.name", baseRefName),
			attribute.String("vcs.ref.base.type", baseRefType),
		)
	}
	return sdkres.NewSchemaless(attrs...)
}

// The ref of a merge queue entry: gh-readonly-queue/<base branch>/pr-<number>-<sha>
var mergeQueueRefName = regexp.MustCompile(`^gh-readonly-queue/(.+)/pr-\d+-[0-9a-f]+$`)

// Returns the name and type of the ref the code lands in: the target branch of a pull request or of
// a merge queue entry, otherwise the ref itself. Unlike GITHUB_REF_NAME ("123/merge" for a pull
// request) it groups the runs of all the changes made to a branch.
func baseRef() (name string, refType string) {
	if baseRef := os.Getenv("GITHUB_BASE_REF"); baseRef != "" {
		return baseRef, "branch"
	}

	refName := os.Getenv("GITHUB_REF_NAME")
	if match := mergeQueueRefName.FindStringSubmatch(refName); match != nil {
		return match[1], "branch"
	}
	return refName, os.Getenv("GITHUB_REF_TYPE")
}

//
// GitHub Runner Attributes
//

// The values of RUNNER_ARCH and RUNNER_OS mapped to the ones of host.arch and os.type
var hostArchs = map[string]string{"X86": "x86", "X64": "amd64", "ARM": "arm32", "ARM64": "arm64"}
var osTypes = map[string]string{"Linux": "linux", "Windows": "windows", "macOS": "darwin"}

func NewGitHubRunnerResource() *sdkres.Resource {
	attrs := []attribute.KeyValue{
		envKeyValue("RUNNER_NAME", "cicd.worker.name"),                 // "x16-core-abc12"
		envKeyValue("RUNNER_ENVIRONMENT", "github.runner.environment"), // "github-hosted" or "self-hosted"
		envMappedKeyValue("RUNNER_ARCH", "host.arch", hostArchs),       // "X64" -> "amd64"
		envMappedKeyValue("RUNNER_OS", "os.type", osTypes),             // "Linux" -> "linux"
	}
	return sdkres.NewSchemaless(attrs...)
}

func envKeyValue(envName string, attributeName string) attribute.KeyValue {
	if value, found := os.LookupEnv(envName); found {
		return attribute.String(attributeName, value)
	}
	return attribute.KeyValue{}
}

// Like envKeyValue with the value mapped, an unknown value is kept lowercased
func envMappedKeyValue(envName string, attributeName string, values map[string]string) attribute.KeyValue {
	value, found := os.LookupEnv(envName)
	if !found {
		return attribute.KeyValue{}
	}
	if mapped, known := values[value]; known {
		return attribute.String(attributeName, mapped)
	}
	return attribute.String(attributeName, strings.ToLower(value))
}

// Returns the value of the first environment variable that isn't empty
func firstEnv(envNames ...string) string {
	for _, envName := range envNames {
		if value := os.Getenv(envName); value != "" {
			return value
		}
	}
	return ""
}

func MergeResources(resources ...*sdkres.Resource) (*sdkres.Resource, error) {
	if len(resources) == 0 {
		return &sdkres.Resource{}, nil
	} else if len(resources) == 1 {
		return resources[0], nil
	}

	var err error
	merged := resources[0]
	for _, resource := range resources[1:] {
		merged, err = sdkres.Merge(merged, resource)
		if err != nil {
			return nil, err
		}
	}

	return merged, err
}
