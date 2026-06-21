package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// TestHandlerPropagatesInboundTrace verifies Rule 1 of the
// observability contract: an inbound W3C traceparent header is parsed
// and installed as the parent context of the app's server span, so the
// trace_id seen inside the handler matches the upstream's. This holds
// even in no-op mode (no exporter) — only the propagator is needed.
func TestHandlerPropagatesInboundTrace(t *testing.T) {
	if _, err := Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const wantTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	var gotTraceID string
	var sampled bool
	wrapped := Handler("test-app", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sc := trace.SpanContextFromContext(r.Context())
		if sc.IsValid() {
			gotTraceID = sc.TraceID().String()
			sampled = sc.IsSampled()
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/resources/foo", nil)
	// W3C traceparent: version-traceid-spanid-flags (01 = sampled).
	req.Header.Set("traceparent", "00-"+wantTraceID+"-00f067aa0ba902b7-01")

	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	if gotTraceID != wantTraceID {
		t.Fatalf("trace_id not propagated from inbound traceparent: got %q want %q", gotTraceID, wantTraceID)
	}
	if !sampled {
		t.Errorf("inbound sampled flag not honored")
	}
}
