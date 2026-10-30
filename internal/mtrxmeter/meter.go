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

// Bucket boundaries in bytes (memory, file system I/O), from 4 KiB up to 64 GiB doubling at each step
var byteBuckets = func() []float64 {
	buckets := []float64{}
	for size := float64(1 << 12); size <= 1<<36; size *= 2 {
		buckets = append(buckets, size)
	}
	return buckets
}()

// Bucket boundaries for counts (page faults, context switches), 1-2-5 steps from 1 up to 5 billion
var countBuckets = func() []float64 {
	buckets := []float64{}
	for decade := float64(1); decade <= 1e9; decade *= 10 {
		buckets = append(buckets, decade, 2*decade, 5*decade)
	}
	return buckets
}()

// The resource attributes kept on the metrics. Every combination of attribute values is a time
// series, so unlike the span the metrics only get the attributes with a low cardinality: no process
// ids, users or runner names, which are different for each execution.
var metricResourceKeys = map[attribute.Key]bool{
	"vcs.owner.name":          true,
	"vcs.repository.name":     true,
	"vcs.ref.base.name":       true,
	"cicd.pipeline.name":      true,
	"github.event.name":       true,
	"cicd.pipeline.task.name": true,
	"host.arch":               true,
	"os.type":                 true,
}

// The attributes of the metrics of a command execution: the low cardinality resource attributes,
// the name of the command and its exit code.
func metricAttributes(resource *sdkres.Resource, process *mtrxtypes.Process, name string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}
	for _, attr := range resource.Attributes() {
		if metricResourceKeys[attr.Key] {
			attrs = append(attrs, attr)
		}
	}

	return append(
		attrs,
		attribute.String("mtrx.command.name", name),
		attribute.Int("process.exit.code", process.Metrics.ExitCode),
	)
}

// Records the measurements of a command execution under the given name. Each execution is a single
// sample of its measurements, so they are histograms: their distribution over many executions is
// what matters.
func CommandMetrics(ctx context.Context, meter metric.Meter, resource *sdkres.Resource, process *mtrxtypes.Process, name string) error {
	metrics := process.Metrics
	attrs := metricAttributes(resource, process, name)
	withAttributes := func(extras ...attribute.KeyValue) metric.MeasurementOption {
		return metric.WithAttributes(append(extras, attrs...)...)
	}

	duration, err := meter.Float64Histogram(
		"mtrx.command.duration",
		metric.WithDescription("Wall clock time of the command"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(timeBuckets...),
	)
	if err != nil {
		return err
	}
	duration.Record(ctx, metrics.Time.Clock, withAttributes())

	cpuTime, err := meter.Float64Histogram(
		"mtrx.command.cpu.time",
		metric.WithDescription("CPU time of the command broken down by mode"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(timeBuckets...),
	)
	if err != nil {
		return err
	}
	cpuTime.Record(ctx, metrics.Time.User, withAttributes(attribute.String("cpu.mode", "user")))
	cpuTime.Record(ctx, metrics.Time.Sys, withAttributes(attribute.String("cpu.mode", "system")))

	memory, err := meter.Int64Histogram(
		"mtrx.command.memory.max",
		metric.WithDescription("Maximum resident set size of the command"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(byteBuckets...),
	)
	if err != nil {
		return err
	}
	// `rusage` has the data in kilobytes
	memory.Record(ctx, metrics.Sys.MaxRss*1024, withAttributes())

	counts := []struct {
		name        string
		description string
		unit        string
		buckets     []float64
		key         string
		values      []countValue
	}{
		{
			"mtrx.command.page_faults", "Page faults of the command broken down by type", "{fault}", countBuckets, "system.paging.fault.type",
			[]countValue{{"minor", metrics.Sys.PageFaults.Minor}, {"major", metrics.Sys.PageFaults.Major}},
		},
		{
			"mtrx.command.context_switches", "Context switches of the command broken down by type", "{switch}", countBuckets, "process.context_switch.type",
			[]countValue{{"voluntary", metrics.ContextSwitch.Voluntarily}, {"involuntary", metrics.ContextSwitch.Involuntarily}},
		},
		{
			"mtrx.command.fs.io", "Bytes read from and written to the storage by the command", "By", byteBuckets, "disk.io.direction",
			[]countValue{{"read", metrics.FsIO.Read}, {"write", metrics.FsIO.Write}},
		},
	}
	for _, count := range counts {
		histogram, err := meter.Int64Histogram(
			count.name,
			metric.WithDescription(count.description),
			metric.WithUnit(count.unit),
			metric.WithExplicitBucketBoundaries(count.buckets...),
		)
		if err != nil {
			return err
		}
		for _, value := range count.values {
			histogram.Record(ctx, value.value, withAttributes(attribute.String(count.key, value.name)))
		}
	}

	return nil
}

// A count measurement and the value of the attribute that distinguishes it
type countValue struct {
	name  string
	value int64
}

// The resource usage counts of a command execution, they annotate its span besides being metrics.
func UsageAttributes(process *mtrxtypes.Process) []attribute.KeyValue {
	metrics := process.Metrics
	return []attribute.KeyValue{
		attribute.Int64("mtrx.page_faults.minor", metrics.Sys.PageFaults.Minor),
		attribute.Int64("mtrx.page_faults.major", metrics.Sys.PageFaults.Major),
		attribute.Int64("mtrx.context_switches.voluntary", metrics.ContextSwitch.Voluntarily),
		attribute.Int64("mtrx.context_switches.involuntary", metrics.ContextSwitch.Involuntarily),
		attribute.Int64("mtrx.fs.read_bytes", metrics.FsIO.Read),
		attribute.Int64("mtrx.fs.write_bytes", metrics.FsIO.Write),
	}
}
