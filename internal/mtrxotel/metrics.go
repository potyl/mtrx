package mtrxotel

import (
	"context"
	"errors"

	otelhttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelconsole "go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkres "go.opentelemetry.io/otel/sdk/resource"
)

// Creates the proper OpenTelemetry exported based on the environment variables:
// OTEL_METRICS_EXPORTER:
//   - console: a console based exporter
//   - otlp: an HTTP based exporter
func newMetricExporter(ctx context.Context) (sdkmetric.Exporter, error) {
	exporterType := getEnvExporter(telemetry_metrics)
	switch exporterType {
	case "console":
		return otelconsole.New(otelconsole.WithPrettyPrint())
	case "otlp":
		return otelhttp.New(ctx)
	default:
		return nil, errors.New("Unknown OpenTelemetry exporter " + exporterType)
	}
}

// OpenTelemetry MeterProvider
func newMeterProvider(reader sdkmetric.Reader, resource *sdkres.Resource) *sdkmetric.MeterProvider {
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(resource),
		sdkmetric.WithReader(reader),
	)
}
