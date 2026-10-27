package mtrxotel

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel/propagation"
)

// Propagates the trace context (TRACEPARENT, TRACESTATE) and the baggage (BAGGAGE) through environment variables.
// See https://opentelemetry.io/docs/specs/otel/context/env-carriers/
var propagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

// A TextMapCarrier that reads the propagation fields from the environment variables.
// The field names are mapped to their upper case equivalent: traceparent -> TRACEPARENT.
type envCarrier struct{}

func (envCarrier) Get(key string) string {
	return os.Getenv(strings.ToUpper(key))
}

func (envCarrier) Set(key string, value string) {
	// The carrier is read only, the child's environment is built by EnvironWithContext
}

func (envCarrier) Keys() []string {
	return propagator.Fields()
}

// Returns a context with the parent span defined by the environment variables, if any.
// This allows the spans of independent mtrx invocations to be part of the same trace.
func ContextFromEnv(ctx context.Context) context.Context {
	return propagator.Extract(ctx, envCarrier{})
}

// Returns the current process' environment where the propagation variables are replaced by the
// values of the given context. This way the spans of the child process are children of our span.
func EnvironWithContext(ctx context.Context) []string {
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)

	propagationVars := map[string]bool{}
	for _, field := range propagator.Fields() {
		propagationVars[strings.ToUpper(field)] = true
	}

	env := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !propagationVars[name] {
			env = append(env, entry)
		}
	}

	for key, value := range carrier {
		env = append(env, strings.ToUpper(key)+"="+value)
	}

	return env
}
