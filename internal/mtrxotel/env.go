package mtrxotel

import (
	"os"
	"strings"
)

type telemetryType string

const (
	telemetry_traces  telemetryType = "TRACES"
	telemetry_metrics telemetryType = "METRICS"
)

func getEnvExporter(telemetry telemetryType) string {
	envNames := []string{
		"OTEL_" + string(strings.ToUpper(string(telemetry))) + "_EXPORTER",
		"OTEL_EXPORTER",
	}

	for _, envName := range envNames {
		exporterType, found := os.LookupEnv(envName)
		if found {
			return exporterType
		}
	}

	return "console"
}
