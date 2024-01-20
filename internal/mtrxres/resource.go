package mtrxres

import (
	"mtrx/internal/mtrxtypes"
	"os"
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
		semconv.ProcessCommandArgs(strings.Join(process.CommandArgs, " ")),
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

func NewGitHubResource() *sdkres.Resource {
	attrs := []attribute.KeyValue{
		envKeyValue("GITHUB_REPOSITORY", "github.repository"), // "org/repo"
		envKeyValue("GITHUB_REF_TYPE", "github.ref.type"),     // "branch" or "tag"
		envKeyValue("GITHUB_REF_NAME", "github.ref.name"),     // "main"
		envKeyValue("GITHUB_REF", "github.ref"),               // "refs/heads/main"
		envKeyValue("GITHUB_JOB", "github.job"),               // "test"
		envKeyValue("GITHUB_ACTOR", "github.actor"),           // "username"
		envKeyValue("GITHUB_EVENT_NAME", "github.event.name"), // "workflow_dispatch"
	}
	return sdkres.NewSchemaless(attrs...)
}

//
// GitHub Runner Attributes
//

func NewGitHubRunnerResource() *sdkres.Resource {
	attrs := []attribute.KeyValue{
		envKeyValue("RUNNER_ARCH", "runner.arch"),               // "Linux"
		envKeyValue("RUNNER_ENVIRONMENT", "runner.environment"), // "X64" or "ARM64"
		envKeyValue("RUNNER_NAME", "runner.name"),               // "github-hosted"
		envKeyValue("RUNNER_OS", "runner.os"),                   // "GitHub Actions 2"
		envKeyValue("RUNNER_USER", "runner.user"),               // "runner"
	}
	return sdkres.NewSchemaless(attrs...)
}

func envKeyValue(envName string, attributeName string) attribute.KeyValue {
	if value, found := os.LookupEnv(envName); found {
		return attribute.String(attributeName, value)
	}
	return attribute.KeyValue{}
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
