package telemetry

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Handler wraps an http.Handler with OTEL server instrumentation
// (Rule 1 of the observability contract). It reads the inbound
// `traceparent` / `tracestate` headers via the global propagator
// installed by Init, makes them the parent context, and starts a
// server span per request — so File Browser's spans stitch into the
// upstream trace (edge → reverse proxy → app) instead of orphaning at
// the proxy→app boundary.
//
// When Init ran in no-op mode the global TracerProvider is the SDK
// default (noop), so otelhttp yields near-zero-overhead noop spans —
// the propagator still parses the header, keeping the wire contract
// intact without an exporter.
//
// serviceName is used as the base span name; empty falls back to
// DefaultServiceName.
func Handler(serviceName string, next http.Handler) http.Handler {
	if serviceName == "" {
		serviceName = DefaultServiceName
	}
	return otelhttp.NewHandler(next, serviceName,
		// Name spans after the request method + path rather than a
		// single static operation name, so the waterfall view shows
		// "GET /api/resources/..." instead of every span sharing one
		// label.
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
}
