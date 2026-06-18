package natsx

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TestHeaderRoundTrip proves trace context injected into NATS headers on the
// publish side is recoverable on the consume side — the foundation of
// end-to-end correlation across service boundaries.
func TestHeaderRoundTrip(t *testing.T) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)

	// Producer side: start a span and inject its context into headers.
	ctx, span := tp.Tracer("test").Start(context.Background(), "produce")
	wantTrace := span.SpanContext().TraceID()
	if !wantTrace.IsValid() {
		t.Fatal("expected a valid trace id on the producer span")
	}
	h := nats.Header{}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(h))
	span.End()

	if h.Get("traceparent") == "" {
		t.Fatal("traceparent header was not injected")
	}

	// Consumer side: a fresh context must recover the same trace id.
	got := trace.SpanContextFromContext(Extract(context.Background(), h)).TraceID()
	if got != wantTrace {
		t.Fatalf("trace id mismatch: got %s want %s", got, wantTrace)
	}
}
