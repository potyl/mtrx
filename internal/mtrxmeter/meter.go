package mtrxmeter

import (
	"context"
	"mtrx/internal/mtrxtypes"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
)

func withAttributes(resource *sdkres.Resource, extras ...attribute.KeyValue) metric.MeasurementOption {
	attributes := resource.Attributes()
	attributes = append(attributes, extras...)
	return metric.WithAttributes(attributes...)
}

func OsProcessMetrics(ctx context.Context, meter metric.Meter, resource *sdkres.Resource, process *mtrxtypes.Process) error {
	metrics := process.Metrics

	if meter, err := meter.Float64Counter(
		"process.cpu.time",
		metric.WithDescription("Total CPU seconds broken down by different states"),
		metric.WithUnit("s"),
	); err == nil {
		meter.Add(ctx, metrics.Time.Sys, withAttributes(resource, attribute.String("state", "system")))
		meter.Add(ctx, metrics.Time.User, withAttributes(resource, attribute.String("state", "user")))
		meter.Add(ctx, metrics.Time.Clock, withAttributes(resource, attribute.String("state", "wait")))
	} else {
		return err
	}

	if meter, err := meter.Int64UpDownCounter(
		"process.memory.usage",
		metric.WithDescription("The amount of physical memory in use"),
		metric.WithUnit("By"),
	); err == nil {
		meter.Add(ctx, metrics.Sys.MaxRss, withAttributes(resource))
	} else {
		return err
	}

	if _, err := meter.Int64ObservableGauge(
		"process.disk.io",
		metric.WithDescription("Disk bytes transferred"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
			observer.Observe(
				metrics.FsBlocks.Input,
				withAttributes(resource, attribute.String("direction", "read")),
			)
			observer.Observe(
				metrics.FsBlocks.Output,
				withAttributes(resource, attribute.String("direction", "write")),
			)
			return nil
		}),
	); err != nil {
		return err
	}

	if meter, err := meter.Int64Counter(
		"process.context_switches",
		metric.WithDescription("Number of times the process has been context switched"),
		metric.WithUnit("{count}"),
	); err == nil {
		meter.Add(
			ctx,
			metrics.ContextSwitch.Involuntarily,
			withAttributes(resource, attribute.String("type", "involuntary")),
		)
		meter.Add(
			ctx,
			metrics.ContextSwitch.Voluntarily,
			withAttributes(resource, attribute.String("type", "voluntary")),
		)
	} else {
		return err
	}

	if meter, err := meter.Int64Counter(
		"process.paging.faults",
		metric.WithDescription("Number of page faults the process has made"),
		metric.WithUnit("{fault}"),
	); err == nil {
		meter.Add(
			ctx,
			metrics.Sys.PageFaults.Minor,
			withAttributes(resource, attribute.String("type", "minor")),
		)
		meter.Add(
			ctx,
			metrics.Sys.PageFaults.Major,
			withAttributes(resource, attribute.String("type", "major")),
		)
	} else {
		return err
	}

	return nil
}
