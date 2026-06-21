package obs

import (
	"os"
	"strings"
)

// Config controls how the observability runtime is wired. Prefer ConfigFromEnv;
// override individual fields only when a service needs to.
type Config struct {
	// ServiceName labels every span, log line, and Sentry event. Required.
	ServiceName string
	// Env is the deployment environment (e.g. "production", "staging", "dev").
	Env string
	// LogLevel is one of debug|info|warn|error. Defaults to info.
	LogLevel string
	// LogFormat is json|text. Defaults to json (text is handy in local dev).
	LogFormat string
	// SentryDSN points at Sentry/Bugsink. Empty disables error reporting.
	SentryDSN string
	// OTLPEndpoint, when set, enables exporting spans to an OTLP/gRPC collector
	// (e.g. "alloy:4317"). Empty (Phase 1) keeps trace IDs flowing into logs
	// without shipping spans anywhere — zero collector traffic.
	OTLPEndpoint string
}

// ConfigFromEnv reads the standard env vars used across all NaraChat services:
//
//	SERVICE_NAME, ENV, LOG_LEVEL, LOG_FORMAT, SENTRY_DSN, OTEL_EXPORTER_OTLP_ENDPOINT
func ConfigFromEnv(serviceName string) Config {
	name := os.Getenv("SERVICE_NAME")
	if name == "" {
		name = serviceName
	}
	ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	ep = strings.TrimPrefix(ep, "http://")
	ep = strings.TrimPrefix(ep, "https://")
	return Config{
		ServiceName:  name,
		Env:          envOr("ENV", "production"),
		LogLevel:     os.Getenv("LOG_LEVEL"),
		LogFormat:    os.Getenv("LOG_FORMAT"),
		SentryDSN:    os.Getenv("SENTRY_DSN"),
		OTLPEndpoint: ep,
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
