# obs-service

Shared observability runtime for all NaraChat Go services. One module gives you
structured logging, tracing, error reporting (Sentry/Bugsink), and — most
importantly — a single `trace_id` that flows across HTTP **and** NATS so the chat
message flow stops being a black box.

`go.mod`: `github.com/gusmanu/obs-service` (public, so CI/CD pulls it with no auth).

## Install

```bash
go get github.com/gusmanu/obs-service@latest
```

## Packages

| Import | Purpose |
| --- | --- |
| `.../obs` | `obs.Init` (wire everything) + `obs.L(ctx)` (trace-correlated logger) |
| `.../natsx` | `natsx.Publish` / `natsx.StartConsumerSpan` — trace context over NATS headers |
| `.../httpx` | `httpx.Middleware` (Gin) / `httpx.Wrap` (net/http) |
| `.../metricx` | Prometheus `/metrics` — `Handler()` for HTTP services, `StartServer()` for workers |

## 1. Bootstrap (every service `main`)

```go
shutdown, err := obs.Init(ctx, obs.ConfigFromEnv("chat-main-service"))
if err != nil { log.Fatal(err) }
defer shutdown(context.Background())
```

Reads env: `SERVICE_NAME`, `ENV`, `LOG_LEVEL`, `LOG_FORMAT`, `SENTRY_DSN`,
`OTEL_EXPORTER_OTLP_ENDPOINT`. Leave `OTEL_EXPORTER_OTLP_ENDPOINT` unset (Phase 1)
to keep `trace_id` in logs without shipping spans anywhere.

## 2. Log with correlation

```go
obs.L(ctx).Info("message persisted", "conversation_id", convID)
obs.L(ctx).Error("dispatch failed", "error", err) // also sent to Sentry/Bugsink
```

Every line auto-gains `trace_id`, `span_id`, and baggage fields (`app_id`,
`conversation_id`, `channel`). Error-level lines are forwarded to Sentry/Bugsink.

## 3. Propagate across NATS

```go
// Producer — replaces js.Publish(subject, data):
natsx.Publish(ctx, natsconn.JS, subject, eventStruct)

// Consumer — first lines of the handler:
ctx, end := natsx.StartConsumerSpan(ctx, msg)
defer end()
```

Event payload structs are unchanged — context travels in NATS headers only, so
byte-compatible contracts between services stay intact.

## 4. HTTP middleware

```go
// Gin:
router.Use(httpx.Middleware("chat-main-service")...)
router.GET("/metrics", gin.WrapH(metricx.Handler()))

// net/http (websocket-service):
http.Handle("/ws", httpx.Wrap("websocket-service", wsHandler))
```

## 5. Metrics for workers (no HTTP server)

```go
metricx.StartServer(":9100")
jobs := metricx.NewCounter("ai_replies_generated_total", "AI replies generated")
```

## Versioning

Tag releases (`vMAJOR.MINOR.PATCH`); services pin via `go get
github.com/gusmanu/obs-service@vX.Y.Z`. Dependency versions are aligned with
`chat-main-service` (otel v1.38.0, sentry v0.34.1, gin v1.10.1) so importing this
module does not force toolchain or dependency upgrades.
