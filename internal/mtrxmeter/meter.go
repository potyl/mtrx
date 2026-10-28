package mtrxmeter

import (
	"context"
	"mtrx/internal/mtrxtypes"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
)

// Bucket boundaries in seconds, from sub-second commands up to the hour long ones of a CI pipeline
var timeBuckets = []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30, 60, 120, 300, 600, 900, 1200, 1800, 2700, 3600, 7200}

// Bucket boundaries in bytes, from 1 MiB up to 32 GiB doubling at each step
var memoryBuckets = func() []float64 {
	buckets := []float64{}
	for size := float64(1 << 20); size <= 1<<35; size *= 2 {
		buckets = append(buckets, size)
	}
	return buckets
}()

func withAttributes(resource *sdkres.Resource, extras ...attribute.KeyValue) metric.MeasurementOption {
	attributes := resource.Attributes()
	attributes = append(attributes, extras...)
	return metric.WithAttributes(attributes...)
}

// Records the measurements of a command execution. Each execution is a single sample of its
// measurements, so they are histograms: their distribution over many executions is what matters.
func CommandMetrics(ctx context.Context, meter metric.Meter, resource *sdkres.Resource, process *mtrxtypes.Process) error {
	metrics := process.Metrics

	duration, err := meter.Float64Histogram(
		"mtrx.command.duration",
		metric.WithDescription("Wall clock time of the command"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(timeBuckets...),
	)
	if err != nil {
		return err
	}
	duration.Record(ctx, metrics.Time.Clock, withAttributes(resource))

	cpuTime, err := meter.Float64Histogram(
		"mtrx.command.cpu.time",
		metric.WithDescription("CPU time of the command broken down by mode"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(timeBuckets...),
	)
	if err != nil {
		return err
	}
	cpuTime.Record(ctx, metrics.Time.User, withAttributes(resource, attribute.String("cpu.mode", "user")))
	cpuTime.Record(ctx, metrics.Time.Sys, withAttributes(resource, attribute.String("cpu.mode", "system")))

	memory, err := meter.Int64Histogram(
		"mtrx.command.memory.max",
		metric.WithDescription("Maximum resident set size of the command"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(memoryBuckets...),
	)
	if err != nil {
		return err
	}
	// `rusage` has the data in kilobytes
	memory.Record(ctx, metrics.Sys.MaxRss*1024, withAttributes(resource))

	return nil
}

// The resource usage counts of a command execution. They describe a single execution rather than
// a trend, so they annotate its span instead of being metrics.
func UsageAttributes(process *mtrxtypes.Process) []attribute.KeyValue {
	metrics := process.Metrics
	return []attribute.KeyValue{
		attribute.Int64("mtrx.page_faults.minor", metrics.Sys.PageFaults.Minor),
		attribute.Int64("mtrx.page_faults.major", metrics.Sys.PageFaults.Major),
		attribute.Int64("mtrx.context_switches.voluntary", metrics.ContextSwitch.Voluntarily),
		attribute.Int64("mtrx.context_switches.involuntary", metrics.ContextSwitch.Involuntarily),
		// `rusage` has the data in blocks, a block is usually 512 bytes, but it can change per device
		attribute.Int64("mtrx.fs_blocks.input", metrics.FsBlocks.Input),
		attribute.Int64("mtrx.fs_blocks.output", metrics.FsBlocks.Output),
	}
}
