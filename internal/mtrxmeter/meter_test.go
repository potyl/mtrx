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
		attrs    attribute.Set
		expected float64
	}{
		{"mtrx.command.duration", *attribute.EmptySet(), 42.5},
		{"mtrx.command.cpu.time", attribute.NewSet(attribute.String("cpu.mode", "user")), 30},
		{"mtrx.command.cpu.time", attribute.NewSet(attribute.String("cpu.mode", "system")), 2.5},
	}
	for _, test := range tests {
		histogram, ok := metrics[test.name].(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("%v: expected a float64 histogram, got %T", test.name, metrics[test.name])
		}
		found := false
		for _, point := range histogram.DataPoints {
			if point.Attributes.Equals(&test.attrs) {
				found = true
				if point.Count != 1 || point.Sum != test.expected {
					t.Errorf("%v %v: got count %v and sum %v, expected a single %v", test.name, test.attrs.Encoded(attribute.DefaultEncoder()), point.Count, point.Sum, test.expected)
				}
			}
		}
		if !found {
			t.Errorf("%v: no data point with the attributes %v", test.name, test.attrs.Encoded(attribute.DefaultEncoder()))
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

func TestMemoryBuckets(t *testing.T) {
	if first, last := memoryBuckets[0], memoryBuckets[len(memoryBuckets)-1]; first != 1<<20 || last != 1<<35 {
		t.Errorf("got buckets from %v to %v, expected from 1 MiB to 32 GiB", first, last)
	}
}
