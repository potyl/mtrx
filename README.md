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
