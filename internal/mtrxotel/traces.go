package mtrxotel

import (
	"context"
	"errors"

	otelhttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelconsole "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Creates the proper OpenTelemetry exported based on the environment variables:
// OTEL_TRACES_EXPORTER:
//   - console: a console based exporter
//   - otlp: an HTTP based exporter
//
// OTEL_EXPORTER_OTLP_ENDPOINT: the endpoint were to send the traces, format scheme://host[:port] (otlp only)
// OTEL_EXPORTER_OTLP_INSECURE: indicates that the endpoint to use doesn't use SSL (otlp only)
func newTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	exporterType := getEnvExporter(telemetry_traces)
	switch exporterType {
	case "console":
		return otelconsole.New(otelconsole.WithPrettyPrint())
	case "otlp":
		return otelhttp.New(ctx)
	default:
		return nil, errors.New("Unknown OpenTelemetry exporter " + exporterType)
	}
}

// OpenTelemetry TracerProvider
func newTracerProvider(exporter sdktrace.SpanExporter, resource *sdkres.Resource) *sdktrace.TracerProvider {
	// This span processor sends the data in realtime
	spanProcessor := sdktrace.NewSimpleSpanProcessor(exporter)

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(spanProcessor),
	)
}
