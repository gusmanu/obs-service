package obs

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
)

// base is the configured root handler (JSON/text + level). L(ctx) wraps it per
// call to inject trace context and forward errors to Sentry.
var (
	base          slog.Handler
	sentryForward bool
)

func initLogger(cfg Config, sentryEnabled bool) {
	level := parseLevel(cfg.LogLevel)
	opts := &slog.HandlerOptions{Level: level}

	if strings.ToLower(cfg.LogFormat) == "text" {
		base = slog.NewTextHandler(os.Stdout, opts)
	} else {
		base = slog.NewJSONHandler(os.Stdout, opts)
	}
	sentryForward = sentryEnabled

	// Default logger carries service name; ctx-specific fields come via L(ctx).
	slog.SetDefault(slog.New(base).With("service", cfg.ServiceName))
}

// L returns a logger bound to ctx. Every record gains trace_id/span_id (when a
// span is active) plus selected baggage fields, and any record at Error level
// or above is forwarded to Sentry/Bugsink. This is the single entry point for
// logging — call obs.L(ctx).Info("...", "key", val) everywhere.
func L(ctx context.Context) *slog.Logger {
	h := base
	if h == nil { // Init not called (e.g. early boot/tests) — safe fallback.
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	return slog.New(&ctxHandler{base: h, ctx: ctx})
}

// ctxHandler binds a context so plain logger.Info/Error calls still see trace
// data and trigger Sentry forwarding without callers threading ctx manually.
type ctxHandler struct {
	base slog.Handler
	ctx  context.Context
}

func (h *ctxHandler) Enabled(_ context.Context, l slog.Level) bool {
	return h.base.Enabled(h.ctx, l)
}

func (h *ctxHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &ctxHandler{base: h.base.WithAttrs(as), ctx: h.ctx}
}

func (h *ctxHandler) WithGroup(name string) slog.Handler {
	return &ctxHandler{base: h.base.WithGroup(name), ctx: h.ctx}
}

func (h *ctxHandler) Handle(_ context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(h.ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	// Promote a few well-known baggage keys to first-class log fields so the
	// chat flow (app/conversation/channel) is filterable in Loki.
	for _, key := range baggageKeys {
		if m := baggage.FromContext(h.ctx).Member(key); m.Value() != "" {
			r.AddAttrs(slog.String(key, m.Value()))
		}
	}

	if sentryForward && r.Level >= slog.LevelError {
		forwardToSentry(h.ctx, r)
	}
	return h.base.Handle(h.ctx, r)
}

var baggageKeys = []string{"app_id", "conversation_id", "channel"}

func forwardToSentry(ctx context.Context, r slog.Record) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			scope.SetTag("trace_id", sc.TraceID().String())
		}
		// If an attribute named "error" carries an error value, capture it as an
		// exception (with stack); otherwise capture the log message.
		var captured error
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "error" {
				if err, ok := a.Value.Any().(error); ok {
					captured = err
					return false
				}
				captured = errors.New(a.Value.String())
				return false
			}
			return true
		})
		if captured != nil {
			scope.SetExtra("log_message", r.Message)
			hub.CaptureException(captured)
		} else {
			hub.CaptureMessage(r.Message)
		}
	})
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
