// Package metricx exposes Prometheus metrics over a /metrics endpoint. HTTP
// services can mount Handler() on their router; workers (no HTTP server) call
// StartServer to spin up a dedicated metrics listener.
package metricx

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is the shared registry. Register custom collectors here so they are
// exported alongside the default Go runtime/process metrics.
var Registry = prometheus.NewRegistry()

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// MustRegister adds collectors to the shared registry, panicking on conflict.
func MustRegister(cs ...prometheus.Collector) { Registry.MustRegister(cs...) }

// Handler serves the registry in Prometheus exposition format.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}

// StartServer runs a minimal HTTP server exposing /metrics on addr (e.g.
// ":9100"). Intended for worker services that have no HTTP server of their own.
// Non-blocking: it launches a goroutine and logs on exit.
func StartServer(addr string) {
	h := http.Handler(Handler())
	start(addr, h)
}

// StartServerWithHandler is like StartServer but wraps the /metrics handler
// with h (e.g. httpx.Wrap for OpenTelemetry tracing of scrape requests).
func StartServerWithHandler(addr string, wrap func(http.Handler) http.Handler) {
	h := wrap(Handler())
	start(addr, h)
}

func start(addr string, h http.Handler) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", h)
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server stopped", "error", err, "addr", addr)
		}
	}()
}

// NewCounter registers and returns a counter. Thin helper so call sites stay
// one-liners instead of building Opts structs everywhere.
func NewCounter(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	Registry.MustRegister(c)
	return c
}

// NewHistogram registers and returns a histogram (default RED-style buckets).
func NewHistogram(name, help string, labels ...string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: prometheus.DefBuckets,
	}, labels)
	Registry.MustRegister(h)
	return h
}
