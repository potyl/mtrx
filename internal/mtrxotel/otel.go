package mtrxotel

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

type OpenTelemetry struct {
	// Traces
	tracerProvider *sdktrace.TracerProvider
	spanExporter   sdktrace.SpanExporter
	tracer         trace.Tracer

	// Metrics
	metricsExporter sdkmetric.Exporter
	metricsReader   sdkmetric.Reader
	meterProvider   *sdkmetric.MeterProvider
	meter           metric.Meter
}

// Creates the OpenTelemetry objects, the given attributes are added to the resource that identifies the app.
func New(ctx context.Context, serviceName string, attrs ...attribute.KeyValue) (*OpenTelemetry, error) {
	// Resources to identify the app
	attrs = append(
		attrs,
		// See https://opentelemetry.io/docs/specs/semconv/resource/#service
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion("0.0.1"),
	)
	resource, err := sdkres.Merge(
		sdkres.Default(),
		sdkres.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	if err != nil {
		return nil, err
	}

	// Traces
	traceExporter, err := newTraceExporter(ctx)
	if err != nil {
		return nil, err
	}

	tracerProvider := newTracerProvider(traceExporter, resource)
	tracer := tracerProvider.Tracer("")
	otel.SetTracerProvider(tracerProvider)

	// Metrics
	metricsExporter, err := newMetricExporter((ctx))
	if err != nil {
		return nil, err
	}
	metricsReader := sdkmetric.NewManualReader()
	meterProvider := newMeterProvider(metricsReader, resource)
	meter := meterProvider.Meter((""))
	otel.SetMeterProvider(meterProvider)

	// Main object
	openTelemetry := OpenTelemetry{
		// Traces
		spanExporter:   traceExporter,
		tracerProvider: tracerProvider,
		tracer:         tracer,

		// Metrics
		metricsExporter: metricsExporter,
		metricsReader:   metricsReader,
		meterProvider:   meterProvider,
		meter:           meter,
	}

	return &openTelemetry, nil
}

func (openTelemetry *OpenTelemetry) exportMetrics(ctx context.Context) error {
	var collectedMetrics metricdata.ResourceMetrics
	if err := openTelemetry.metricsReader.Collect(ctx, &collectedMetrics); err != nil {
		return fmt.Errorf("metrics collect: %w", err)
	}
	if err := openTelemetry.metricsExporter.Export(ctx, &collectedMetrics); err != nil {
		return fmt.Errorf("metrics export: %w", err)
	}
	return nil
}

func (openTelemetry *OpenTelemetry) Shutdown(ctx context.Context) (err error) {
	// We need to export the metrics by hand
	err = openTelemetry.exportMetrics(ctx)

	shutdowns := []func(context.Context) error{
		openTelemetry.tracerProvider.Shutdown,
		openTelemetry.spanExporter.Shutdown,
		openTelemetry.metricsReader.Shutdown,
		openTelemetry.metricsExporter.Shutdown,
	}

	for _, shutdown := range shutdowns {
		err = errors.Join(err, shutdown(ctx))
	}

	return err
}

func (openTelemetry *OpenTelemetry) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return openTelemetry.tracer.Start(ctx, name, opts...)
}

func (openTelemetry *OpenTelemetry) Meter() metric.Meter {
	return openTelemetry.meter
}
