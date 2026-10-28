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

Each execution sends a span and the following metrics. An execution is a single sample, so they are histograms: they
give the distribution of the measurements over many executions.

| Metric                    | Unit | Description                                       |
| ------------------------- | ---- | ------------------------------------------------- |
| `mtrx.command.duration`   | `s`  | Wall clock time of the command                    |
| `mtrx.command.cpu.time`   | `s`  | CPU time, by `cpu.mode`: `user` or `system`       |
| `mtrx.command.memory.max` | `By` | Maximum resident set size                         |

The utility is expected to be executed from CI, mainly GitHub and the data collected is annotated with GitHub's CI information:

| Environment variable | Attribute            | On the metrics |
| -------------------- | -------------------- | -------------- |
| GITHUB_REPOSITORY    | `github.repository`  | yes            |
| GITHUB_REF_TYPE      | `github.ref.type`    |                |
| GITHUB_REF_NAME      | `github.ref.name`    |                |
| GITHUB_REF           | `github.ref`         |                |
| GITHUB_JOB           | `github.job`         | yes            |
| GITHUB_ACTOR         | `github.actor`       |                |
| GITHUB_EVENT_NAME    | `github.event.name`  | yes            |
| RUNNER_ARCH          | `runner.arch`        | yes            |
| RUNNER_ENVIRONMENT   | `runner.environment` |                |
| RUNNER_NAME          | `runner.name`        |                |
| RUNNER_OS            | `runner.os`          | yes            |
| RUNNER_USER          | `runner.user`        |                |

`vcs.ref.base.name` is the branch the code lands in: the target branch of a pull request (`GITHUB_BASE_REF`) or of a
merge queue entry, otherwise `GITHUB_REF_NAME`. It's on the metrics too, so the executions of a branch can be compared
whether they come from pushes, pull requests or the merge queue.

The metrics also have the command line (`process.command_line`) and its exit code (`process.exit.code`). Every
combination of attribute values is a time series, so the metrics only get the attributes with a low cardinality. The
span gets all of them, plus the process ids and the page faults, context switches and file system blocks.

For instance the median duration of a build on `main` over a week:

```promql
histogram_quantile(0.5, sum by (le) (increase(mtrx_command_duration_bucket{
  github_repository="cloudbeds/mfd", vcs_ref_base_name="main", process_command_line="docker compose build"}[1w])))
```

### Configuration

The program can be configured through the standard OpenTelemetry environment variables:

```bash
export OTEL_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=http://opentelemetry-collector.opentelemetry.svc:4318
```

Each `mtrx` process sends the metrics of a single execution. Sent as cumulative values (the default), the executions
with the same attributes look like a series that restarts each time. Send them as deltas instead:

```bash
export OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=delta
```

A backend that only accepts cumulative values, like Prometheus or Mimir, needs a collector adding up the deltas with
the [deltatocumulative](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/deltatocumulativeprocessor)
processor. It keeps the totals in memory, so all the deltas of a series must reach the same collector instance.

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
