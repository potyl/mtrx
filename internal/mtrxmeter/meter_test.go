package mtrxmeter

import (
	"context"
	"testing"

	"mtrx/internal/mtrxtypes"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
)

func newTestProcess() *mtrxtypes.Process {
	metrics := mtrxtypes.Metrics{}
	metrics.Time.Clock = 42.5
	metrics.Time.User = 30
	metrics.Time.Sys = 2.5
	metrics.Sys.MaxRss = 2048
	metrics.Sys.PageFaults.Minor = 300
	metrics.Sys.PageFaults.Major = 4
	metrics.ContextSwitch.Voluntarily = 50
	metrics.ContextSwitch.Involuntarily = 6
	metrics.FsIO.Read = 70 * 512
	metrics.FsIO.Write = 8 * 512
	return &mtrxtypes.Process{Metrics: &metrics, Command: "make", CommandArgs: []string{"build"}}
}

// Records the metrics of the given process and returns them by name
func collect(t *testing.T, resource *sdkres.Resource, process *mtrxtypes.Process) map[string]metricdata.Aggregation {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")

	if err := CommandMetrics(ctx, meter, resource, process); err != nil {
		t.Fatal(err)
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &collected); err != nil {
		t.Fatal(err)
	}

	metrics := map[string]metricdata.Aggregation{}
	for _, scopeMetrics := range collected.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			metrics[metric.Name] = metric.Data
		}
	}
	return metrics
}

func TestCommandMetricsAreHistograms(t *testing.T) {
	metrics := collect(t, sdkres.Empty(), newTestProcess())

	tests := []struct {
		name     string
		cpuMode  string
		expected float64
	}{
		{"mtrx.command.duration", "", 42.5},
		{"mtrx.command.cpu.time", "user", 30},
		{"mtrx.command.cpu.time", "system", 2.5},
	}
	for _, test := range tests {
		histogram, ok := metrics[test.name].(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("%v: expected a float64 histogram, got %T", test.name, metrics[test.name])
		}
		found := false
		for _, point := range histogram.DataPoints {
			if cpuMode, _ := point.Attributes.Value("cpu.mode"); cpuMode.AsString() == test.cpuMode {
				found = true
				if point.Count != 1 || point.Sum != test.expected {
					t.Errorf("%v %v: got count %v and sum %v, expected a single %v", test.name, test.cpuMode, point.Count, point.Sum, test.expected)
				}
			}
		}
		if !found {
			t.Errorf("%v: no data point with the cpu.mode %q", test.name, test.cpuMode)
		}
	}

	memory, ok := metrics["mtrx.command.memory.max"].(metricdata.Histogram[int64])
	if !ok {
		t.Fatalf("mtrx.command.memory.max: expected an int64 histogram, got %T", metrics["mtrx.command.memory.max"])
	}
	if sum := memory.DataPoints[0].Sum; sum != 2048*1024 {
		t.Errorf("mtrx.command.memory.max: got %v, expected the max RSS in bytes %v", sum, 2048*1024)
	}
}

func TestCountMetrics(t *testing.T) {
	metrics := collect(t, sdkres.Empty(), newTestProcess())

	tests := []struct {
		name     string
		key      attribute.Key
		value    string
		expected int64
	}{
		{"mtrx.command.page_faults", "type", "minor", 300},
		{"mtrx.command.page_faults", "type", "major", 4},
		{"mtrx.command.context_switches", "type", "voluntary", 50},
		{"mtrx.command.context_switches", "type", "involuntary", 6},
		{"mtrx.command.fs.io", "direction", "read", 70 * 512},
		{"mtrx.command.fs.io", "direction", "write", 8 * 512},
	}
	for _, test := range tests {
		histogram, ok := metrics[test.name].(metricdata.Histogram[int64])
		if !ok {
			t.Fatalf("%v: expected an int64 histogram, got %T", test.name, metrics[test.name])
		}
		found := false
		for _, point := range histogram.DataPoints {
			if value, _ := point.Attributes.Value(test.key); value.AsString() == test.value {
				found = true
				if point.Count != 1 || point.Sum != test.expected {
					t.Errorf("%v %v: got count %v and sum %v, expected a single %v", test.name, test.value, point.Count, point.Sum, test.expected)
				}
			}
		}
		if !found {
			t.Errorf("%v: no data point with %v=%q", test.name, test.key, test.value)
		}
	}
}

func TestCountBuckets(t *testing.T) {
	if first, last := countBuckets[0], countBuckets[len(countBuckets)-1]; first != 1 || last != 5e9 {
		t.Errorf("got buckets from %v to %v, expected from 1 to 5e9", first, last)
	}
}

func TestByteBuckets(t *testing.T) {
	if first, last := byteBuckets[0], byteBuckets[len(byteBuckets)-1]; first != 1<<12 || last != 1<<36 {
		t.Errorf("got buckets from %v to %v, expected from 4 KiB to 64 GiB", first, last)
	}
}

func TestMetricAttributesHaveALowCardinality(t *testing.T) {
	resource := sdkres.NewSchemaless(
		attribute.String("vcs.owner.name", "cloudbeds"),
		attribute.String("vcs.repository.name", "mfd"),
		attribute.String("vcs.ref.head.name", "emo/feature"),
		attribute.String("vcs.ref.base.name", "main"),
		attribute.String("cicd.pipeline.task.name", "phpstan"),
		attribute.String("cicd.worker.name", "x16-core-abc"),
		attribute.Int("process.pid", 1234),
		attribute.String("process.owner", "runner"),
	)
	process := newTestProcess()
	process.Metrics.ExitCode = 2

	got := attribute.NewSet(metricAttributes(resource, process)...)
	expected := attribute.NewSet(
		attribute.String("vcs.owner.name", "cloudbeds"),
		attribute.String("vcs.repository.name", "mfd"),
		attribute.String("vcs.ref.base.name", "main"),
		attribute.String("cicd.pipeline.task.name", "phpstan"),
		attribute.String("process.command_line", "make build"),
		attribute.Int("process.exit.code", 2),
	)
	if !got.Equals(&expected) {
		t.Errorf("got %v, expected %v", got.Encoded(attribute.DefaultEncoder()), expected.Encoded(attribute.DefaultEncoder()))
	}
}
