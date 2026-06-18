// Package httpx provides ready-made HTTP instrumentation: a Gin middleware
// stack and a net/http wrapper, both starting a trace and reporting panics to
// Sentry. Use Middleware for Gin services and Wrap for raw net/http
// (websocket-service).
package httpx

import (
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// Middleware returns the standard Gin instrumentation chain for a service:
// OTel span per request (extracts inbound traceparent) + Sentry panic capture.
// Apply with: router.Use(httpx.Middleware("chat-main-service")...).
func Middleware(serviceName string) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		otelgin.Middleware(serviceName),
		sentrygin.New(sentrygin.Options{Repanic: true}),
	}
}
