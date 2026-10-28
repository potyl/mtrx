package mtrxotel

import (
	"context"
	"crypto/rand"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type forcedSpanIDKey struct{}

// Returns a context where the next span started gets the given span id instead of a random one.
// This allows to export spans whose ids were agreed upon beforehand, for instance the parent span
// that other processes already used in their TRACEPARENT.
// To force the trace id of a span without a parent, give ctx a span context with that trace id and
// no span id: the SDK keeps its trace id and the span is still exported without a parent.
func ContextWithSpanID(ctx context.Context, spanID trace.SpanID) context.Context {
	return context.WithValue(ctx, forcedSpanIDKey{}, spanID)
}

// Generates random ids unless the context forces them, see ContextWithSpanID.
type idGenerator struct{}

var _ sdktrace.IDGenerator = idGenerator{}

func (idGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	return randomTraceID(), spanIDFromContext(ctx)
}

func (idGenerator) NewSpanID(ctx context.Context, _ trace.TraceID) trace.SpanID {
	return spanIDFromContext(ctx)
}

func spanIDFromContext(ctx context.Context) trace.SpanID {
	if spanID, ok := ctx.Value(forcedSpanIDKey{}).(trace.SpanID); ok && spanID.IsValid() {
		return spanID
	}
	return randomSpanID()
}

func randomTraceID() (traceID trace.TraceID) {
	for !traceID.IsValid() {
		_, _ = rand.Read(traceID[:])
	}
	return traceID
}

func randomSpanID() (spanID trace.SpanID) {
	for !spanID.IsValid() {
		_, _ = rand.Read(spanID[:])
	}
	return spanID
}
