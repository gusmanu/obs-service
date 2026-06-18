// Package obs is the shared observability runtime for all NaraChat services.
//
// One call to Init wires structured logging (slog), tracing (OpenTelemetry),
// error reporting (Sentry/Bugsink), and the W3C propagators used to carry a
// single trace_id across HTTP and NATS boundaries. Use obs.L(ctx) for logging
// so every line is automatically correlated to its trace.
package obs

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

// Shutdown flushes telemetry. Defer it in main with a bounded context.
type Shutdown func(ctx context.Context) error

// Init bootstraps the observability runtime and returns a Shutdown to defer.
// It is safe to call once at startup; subsequent setup (NATS, HTTP) reuses the
// global tracer/propagator installed here.
func Init(ctx context.Context, cfg Config) (Shutdown, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("obs: ServiceName is required")
	}

	// 1) Structured logging first, so the rest of boot is observable.
	sentryEnabled := cfg.SentryDSN != ""
	initLogger(cfg, sentryEnabled)

	// 2) W3C propagation — the contract that carries trace context across
	//    HTTP headers and NATS message headers.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 3) Tracer provider. Always generates valid trace/span IDs (AlwaysSample)
	//    so logs are correlated even when no collector is configured (Phase 1).
	res, _ := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Env),
		),
	)

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	}

	if cfg.OTLPEndpoint != "" {
		exp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithTimeout(5*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("obs: otlp exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)

	// 4) Sentry / Bugsink — Sentry-SDK compatible, so the same client works.
	if sentryEnabled {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:         cfg.SentryDSN,
			Environment: cfg.Env,
			ServerName:  cfg.ServiceName,
		}); err != nil {
			// Non-fatal: degrade to logs-only error reporting.
			L(ctx).Warn("sentry init failed, continuing without error reporting", "error", err)
			sentryEnabled = false
		}
	}

	return func(shutdownCtx context.Context) error {
		if sentryEnabled {
			sentry.Flush(2 * time.Second)
		}
		return tp.Shutdown(shutdownCtx)
	}, nil
}
