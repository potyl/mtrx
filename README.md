# mtrx

`mtrx` is a simple CLI utility that mimics the [time](https://man7.org/linux/man-pages/man1/time.1.html) command
behavior when executed with `-f` and grabs as the resource usage as exposed by the
[getrusage](https://man7.org/linux/man-pages/man2/getrusage.2.html) system call.

Just as with the `time` utility the program expects a command and its arguments and executes it.
Once the command has resumed the resource usage captured is sent to an OpenTelemetry stack.

This is a standalone program that doesn't require an external daemons or Prometheus exporter.
As long as an OpenTelemetry stack is available and can be pointed to `mtrx` will deliver the metrics on its own.

## Usage

The program is meant to be used as the `time` command.
For instance, to capture metrics from a build command do:

```bash
mtrx make build
```

## OpenTelemetry

### Exporter

The program uses relies on the OpenTelemetry SDK to send metrics.
The metrics are sent in realtime at the end of the command being executed.
No Prometheus exporter is required nor expected.

### Metrics and traces

The command sends metrics and traces.
The utility is expected to be executed from CI, mainly GitHub and the data collected is annotated with GitHub's CI information:

| Environment variable | Label                |
| -------------------- | ------               |
| GITHUB_REF_TYPE      |  `github.ref.type`   |
| GITHUB_REF_NAME      |  `github.ref.name`   |
| GITHUB_REF           |  `github.ref`        |
| GITHUB_JOB           |  `github.job`        |
| GITHUB_ACTOR         |  `github.actor`      |
| GITHUB_EVENT_NAME    |  `github.event.name` |
| RUNNER_ARCH          | `runner.arch`        |
| RUNNER_ENVIRONMENT   | `runner.environment` |
| RUNNER_NAME          | `runner.name`        |
| RUNNER_OS            | `runner.os`          |
| RUNNER_USER          | `runner.user`        |

### Configuration

The program can be configured through the standard OpenTelemetry environment variables:

```bash
export OTEL_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=http://opentelemetry-collector.opentelemetry.svc:4318
```

### Trace propagation

The span's parent is read from the [environment variable carriers](https://opentelemetry.io/docs/specs/otel/context/env-carriers/)
`TRACEPARENT`, `TRACESTATE` and `BAGGAGE`. This links independent `mtrx` invocations into a single trace, for
instance all the commands of a CI run:

```bash
export TRACEPARENT=00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
mtrx docker compose pull
mtrx docker compose up -d
```

The child process receives these variables set to the `mtrx` span, so any instrumented program it runs (including
`mtrx` itself) records its spans as children of the `mtrx` span.

### GitHub Actions

When running in GitHub Actions (`GITHUB_ACTIONS=true`) and `TRACEPARENT` isn't set, `mtrx` links the spans of the
whole workflow run into a single trace without any configuration:

- the trace id is derived from the run attempt (`GITHUB_REPOSITORY`, `GITHUB_RUN_ID`, `GITHUB_RUN_ATTEMPT`)
- the parent span id is derived from the job's runner (`RUNNER_NAME`), as runners are ephemeral it identifies the job
- the resource gets the attributes `cicd.pipeline.run.id`, `cicd.pipeline.run.attempt` and, for a pull request,
  `vcs.change.id` (the PR number taken from `GITHUB_REF`)

These parents only exist once the run is exported, from the last job of the workflow:

```yaml
  ci-trace:
    name: CI Trace
    if: ${{ always() }}
    needs: [all-the-leaf-jobs]
    runs-on: ubuntu-latest
    permissions:
      actions: read
    steps:
      # Install mtrx and set OTEL_EXPORTER and OTEL_EXPORTER_OTLP_ENDPOINT
      - name: Export run trace
        env:
          GITHUB_TOKEN: ${{ github.token }}
        run: mtrx --export-github-run
```

`mtrx --export-github-run` reads the run and its jobs from the GitHub API and exports, under the service `ci`, a root
span for the run and a span per job with their start and end times. A job that didn't succeed gets an error status,
and so does the run.
