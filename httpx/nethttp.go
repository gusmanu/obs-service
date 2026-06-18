package httpx

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Wrap instruments a raw net/http handler with an OTel span per request,
// extracting any inbound traceparent. For services not using Gin
// (e.g. websocket-service):
//
//	http.Handle("/ws", httpx.Wrap("websocket-service", mux))
func Wrap(serviceName string, h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, serviceName)
}
