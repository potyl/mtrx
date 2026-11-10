package mtrxotel

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel/propagation"
)

// Propagates the trace context (TRACEPARENT, TRACESTATE) and the baggage (BAGGAGE) through environment variables.
// See https://opentelemetry.io/docs/specs/otel/context/env-carriers/
var propagators = []propagation.TextMapPropagator{
	propagation.TraceContext{},
	propagation.Baggage{},
}

var propagator = propagation.NewCompositeTextMapPropagator(propagators...)

// The fields in the order of the propagators: the composite propagator's Fields() has no order
func propagationFields() []string {
	fields := []string{}
	for _, p := range propagators {
		fields = append(fields, p.Fields()...)
	}
	return fields
}

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
	return propagationFields()
}

// Returns a context with the parent span defined by the environment variables, if any.
// This allows the spans of independent mtrx invocations to be part of the same trace.
func ContextFromEnv(ctx context.Context) context.Context {
	return propagator.Extract(ctx, envCarrier{})
}

// Returns the propagation variables of the given context (TRACEPARENT=..., then TRACESTATE and
// BAGGAGE when they have a value), in the syntax of an environment file.
func PropagationEnv(ctx context.Context) []string {
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)

	env := []string{}
	for _, field := range propagationFields() {
		if value, ok := carrier[field]; ok {
			env = append(env, strings.ToUpper(field)+"="+value)
		}
	}
	return env
}

// Returns the current process' environment where the propagation variables are replaced by the
// values of the given context. This way the spans of the child process are children of our span.
func EnvironWithContext(ctx context.Context) []string {
	propagationVars := map[string]bool{}
	for _, field := range propagationFields() {
		propagationVars[strings.ToUpper(field)] = true
	}

	env := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !propagationVars[name] {
			env = append(env, entry)
		}
	}

	return append(env, PropagationEnv(ctx)...)
}
