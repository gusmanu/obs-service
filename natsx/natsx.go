// Package natsx carries trace context across NATS boundaries. It injects the
// W3C traceparent/baggage into nats.Msg headers on publish and extracts them on
// consume, so a single trace_id spans channel -> chat-main -> orchestrator and
// back. Event payload structs are unchanged — context rides in headers only.
package natsx

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/gusmanu/obs-service/natsx"

// headerCarrier adapts nats.Header to the OTel TextMapCarrier interface.
type headerCarrier nats.Header

func (c headerCarrier) Get(key string) string { return nats.Header(c).Get(key) }
func (c headerCarrier) Set(key, val string)   { nats.Header(c).Set(key, val) }
func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

var _ propagation.TextMapCarrier = headerCarrier{}

// Publish marshals payload to JSON, injects the active trace context into the
// message headers, and publishes via JetStream. Drop-in replacement for
// js.Publish(subject, data) that adds propagation.
func Publish(ctx context.Context, js nats.JetStreamContext, subject string, payload any) (*nats.PubAck, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("natsx: marshal %s: %w", subject, err)
	}
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(msg.Header))
	return js.PublishMsg(msg)
}

// PublishCore is the non-JetStream variant for plain NATS subjects.
func PublishCore(ctx context.Context, nc *nats.Conn, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("natsx: marshal %s: %w", subject, err)
	}
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(msg.Header))
	return nc.PublishMsg(msg)
}

// Inject writes the active trace context into an existing NATS header map. Use
// it for the PublishMsg-with-options case where Publish (which builds its own
// message) isn't suitable. Initialize the header first: msg.Header = nats.Header{}.
func Inject(ctx context.Context, h nats.Header) {
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(h))
}

// Extract returns ctx enriched with the trace context found in the message
// headers (no-op when absent). Use it when you only need correlation, not a span.
func Extract(ctx context.Context, h nats.Header) context.Context {
	if h == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, headerCarrier(h))
}

// StartConsumerSpan extracts the upstream trace from the message and starts a
// CONSUMER span for handling it. Always call the returned end func (defer).
//
//	ctx, end := natsx.StartConsumerSpan(ctx, m)
//	defer end()
func StartConsumerSpan(ctx context.Context, m *nats.Msg) (context.Context, func()) {
	ctx = Extract(ctx, m.Header)
	ctx, span := otel.Tracer(tracerName).Start(ctx,
		"consume "+m.Subject,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			semconv.MessagingSystemKey.String("nats"),
			semconv.MessagingDestinationNameKey.String(m.Subject),
		),
	)
	return ctx, func() { span.End() }
}
