# mtrx

`mtrx` is a simple CLI utility that mimics the [time](https://man7.org/linux/man-pages/man1/time.1.html) command
behavior when executed with `-f` and grabs as the resource usage as exposed by the
[getrusage](https://man7.org/linux/man-pages/man2/getrusage.2.html) system call.

Just as with the `time` utility the program expects a command and its arguments and executes it.
Once the command has resumed the resource usage captured is sent to an OpenTelemetry stack.

This is a standalone program that doesn't require an external daemons or Prometheus exporter.
As long as an OpenTelemetry stack is available and can be pointed to `mtrx` will deliver the metrics on its own.

## Usage

`mtrx exec` is meant to be used as the `time` command.
For instance, to capture metrics from a build command do:

```bash
mtrx exec make build
```

The flags after the command are the command's, `mtrx exec docker compose build --pull` passes `--pull` to docker.
`mtrx exec` exits with the exit status of the command.

| Command                   | Description                                                      |
| ------------------------- | ---------------------------------------------------------------- |
| `mtrx exec CMD [ARG]...`  | Runs the command and sends its trace and metrics                 |
| `mtrx github export`      | Exports the trace of the current GitHub Actions run, see below   |
| `mtrx version`            | Prints the version, like `mtrx --version`                        |

## OpenTelemetry

### Exporter

The program uses relies on the OpenTelemetry SDK to send metrics.
The metrics are sent in realtime at the end of the command being executed.
No Prometheus exporter is required nor expected.

### Metrics and traces

Each execution sends a span and the following metrics. An execution is a single sample, so they are histograms: they
give the distribution of the measurements over many executions.

| Metric                          | Unit       | Attribute                                         |
| ------------------------------- | ---------- | ------------------------------------------------- |
| `mtrx.command.duration`         | `s`        | Wall clock time of the command                    |
| `mtrx.command.cpu.time`         | `s`        | CPU time, by `cpu.mode`: `user` or `system`       |
| `mtrx.command.memory.max`       | `By`       | Maximum resident set size                         |
| `mtrx.command.page_faults`      | `{fault}`  | By `system.paging.fault.type`: `minor` or `major` |
| `mtrx.command.context_switches` | `{switch}` | By `process.context_switch.type`: `voluntary` or `involuntary` |
| `mtrx.command.fs.io`            | `By`       | Bytes read from and written to the storage, by `disk.io.direction`: `read` or `write` |

The utility is expected to be executed from CI, mainly GitHub Actions. The data is annotated from GitHub's environment
variables (no GitHub API call), with the names of the
[OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/registry/attributes/) when they define
one, under `github.*` otherwise:

| Attribute                    | Source                                                    | Metrics | Traces |
| ---------------------------- | --------------------------------------------------------- | ------- | ----- |
| `vcs.provider.name`          | `github`                                                  |         | yes   |
| `vcs.owner.name`             | `GITHUB_REPOSITORY_OWNER`                                 | yes     | yes   |
| `vcs.repository.name`        | `GITHUB_REPOSITORY` without the owner                     | yes     | yes   |
| `vcs.repository.url.full`    | `GITHUB_SERVER_URL`/`GITHUB_REPOSITORY`                   |         | yes   |
| `vcs.ref.head.name`          | `GITHUB_HEAD_REF` for a pull request, else `GITHUB_REF_NAME` |      | yes   |
| `vcs.ref.head.type`          | `GITHUB_REF_TYPE`                                         |         | yes   |
| `vcs.ref.head.revision`      | `GITHUB_SHA` (the merge commit for a pull request)        |         | yes   |
| `vcs.ref.base.name`          | the branch the code lands in, see below                   | yes     | yes   |
| `vcs.ref.base.type`          | `branch`, or `GITHUB_REF_TYPE` outside of a change        |         | yes   |
| `vcs.change.id`              | the pull request number from `GITHUB_REF`                 |         | yes   |
| `cicd.pipeline.name`         | `GITHUB_WORKFLOW`                                         | yes     | yes   |
| `cicd.pipeline.task.name`    | `GITHUB_JOB`                                              | yes     | yes   |
| `cicd.pipeline.run.id`       | `GITHUB_RUN_ID`                                           |         | yes   |
| `cicd.pipeline.run.url.full` | the URL of the run attempt                                |         | yes   |
| `cicd.worker.name`           | `RUNNER_NAME`                                             |         | yes   |
| `host.arch`                  | `RUNNER_ARCH`: `X64` -> `amd64`, `ARM64` -> `arm64`       | yes     | yes   |
| `os.type`                    | `RUNNER_OS`: `Linux` -> `linux`                           | yes     | yes   |
| `github.event.name`          | `GITHUB_EVENT_NAME`                                       | yes     | yes   |
| `github.actor`               | `GITHUB_ACTOR`                                            |         | yes   |
| `github.run.attempt`         | `GITHUB_RUN_ATTEMPT`                                      |         | yes   |
| `github.runner.environment`  | `RUNNER_ENVIRONMENT`                                      |         | yes   |

`vcs.ref.base.name` is the branch the code lands in: the target branch of a pull request (`GITHUB_BASE_REF`) or of a
merge queue entry, otherwise `GITHUB_REF_NAME`. It's on the metrics, so the executions of a branch can be compared
whether they come from pushes, pull requests or the merge queue.

These attributes are the same for everything a process sends, so on the traces they're resource attributes
(`resource.vcs.repository.name` in TraceQL). The span of a command only has what describes the execution: the
process (`process.command`, `process.command_args`, `process.pid`, ...), its exit code (`process.exit.code`) and its
resource usage (`mtrx.page_faults.*`, `mtrx.context_switches.*`, `mtrx.fs.*_bytes`). A non zero exit code makes the
span an error, with `error.type` set to the exit code.

Every combination of attribute values is a time series, so the metrics only get the attributes with a low cardinality,
as data point attributes, plus the name of the command (`mtrx.command.name`) and its exit code (`process.exit.code`).

The name of the command is its command line, or the name given with `mtrx exec --name`. A command line that changes
from one execution to the next, like a list of test files, would otherwise make each execution a new time series:

```bash
echo "${tests[@]}" | mtrx exec --name "npm run test" xargs npm run test --
```

The span is named after the command too, and keeps the actual command in `process.command` and
`process.command_args`.

`mtrx exec --attribute key=value` adds an attribute to the span and the metrics, it can be repeated. For instance to
tell apart the test suites and shards that run the same command:

```bash
echo "${tests[@]}" | mtrx exec --name "npm run test" \
  --attribute test.suite.name=API --attribute test.suite.shard=2/4 \
  xargs npm run test --
```

Every value of a metric attribute is a time series, so an attribute should only have a few distinct values.

For instance the median duration of a build on `main` over a week:

```promql
histogram_quantile(0.5, sum by (le) (increase(mtrx_command_duration_bucket{
  vcs_owner_name="cloudbeds", vcs_repository_name="mfd", vcs_ref_base_name="main",
  mtrx_command_name="docker compose build"}[1w])))
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
mtrx exec docker compose pull
mtrx exec docker compose up -d
```

The child process receives these variables set to the `mtrx` span, so any instrumented program it runs (including
`mtrx` itself) records its spans as children of the `mtrx` span.

### GitHub Actions

When running in GitHub Actions (`GITHUB_ACTIONS=true`) and `TRACEPARENT` isn't set, `mtrx` links the spans of the
whole workflow run into a single trace without any configuration:

- the trace id is derived from the run attempt (`GITHUB_REPOSITORY`, `GITHUB_RUN_ID`, `GITHUB_RUN_ATTEMPT`)
- the parent span id is derived from the job's runner (`RUNNER_NAME`), as runners are ephemeral it identifies the job
- the resource of the traces gets the attributes `cicd.pipeline.run.id`, `cicd.pipeline.run.url.full`,
  `github.run.attempt` and, for a pull request, `vcs.change.id` (the PR number taken from `GITHUB_REF`)

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
        run: mtrx github export
```

`mtrx github export` reads the run and its jobs from the GitHub API and exports, under the service `github-actions`, a root
span for the run and a span per job with their start and end times. A job that didn't succeed gets an error status,
and so does the run.
