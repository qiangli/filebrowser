// Package telemetry bootstraps the OTEL SDK for File Browser so it
// participates in the cross-process trace fabric described by the
// cooperative-web-apps observability contract: a request that enters
// at the edge (e.g. cloudbox) carries a W3C `traceparent`; each
// reverse-proxy hop re-emits it; by the time it reaches this app the
// header already names a trace_id spanning the upstream hops. Wrapping
// the HTTP handler with Handler() (Rule 1) makes File Browser's own
// server spans children of that trace instead of orphaning at the
// proxy→app boundary.
//
// Configuration is pure-env-var, honoring the standard OTEL spec:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT     gRPC collector, e.g. 127.0.0.1:4317
//	OTEL_SERVICE_NAME               defaults to "filebrowser"
//	OTEL_RESOURCE_ATTRIBUTES        e.g. service.namespace=dhnt,deployment.environment=prod
//	OTEL_TRACES_SAMPLER             parentbased_traceidratio (default)
//	OTEL_TRACES_SAMPLER_ARG         e.g. "1.0" or "0.1"
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is unset the provider runs in
// "no-op" mode: the global TextMapPropagator is still installed so the
// inbound traceparent is parsed (and would flow onto any outbound hop),
// but no TracerProvider or exporter is created. This keeps the runtime
// footprint near-zero for a plain LAN/loopback deployment that hasn't
// enrolled in a collector — the operator opts in by setting one env var.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// DefaultServiceName is the service.name reported when OTEL_SERVICE_NAME
// is unset. Matches the value an operator points OTEL_SERVICE_NAME at,
// and the service.name a cooperative app is expected to use in the
// cross-process waterfall (alongside "cloudbox-hub" and "outpost").
const DefaultServiceName = "filebrowser"

// Provider holds the OTEL SDK lifetime for this process.
type Provider struct {
	TracerProvider *sdktrace.TracerProvider // nil in no-op mode
	shutdown       []func(context.Context) error
}

// Shutdown flushes pending spans and tears down exporters. Safe to call
// on a nil Provider and safe to call multiple times.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	for _, fn := range p.shutdown {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	p.shutdown = nil
	return errors.Join(errs...)
}

var (
	initOnce sync.Once
	initErr  error
	initProv *Provider
)

// Init bootstraps the global OTEL trace provider + propagator. Safe to
// call multiple times — only the first call boots; later calls return
// the same Provider so a restart doesn't double-install exporters.
//
// The W3C propagator is installed even in no-op mode, so the trace
// fabric's wire contract survives whether or not this host exports.
func Init(ctx context.Context) (*Provider, error) {
	initOnce.Do(func() {
		initProv, initErr = build(ctx)
	})
	return initProv, initErr
}

func build(ctx context.Context) (*Provider, error) {
	// Always install the W3C propagator — that's the contract.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// No-op mode: propagator stays installed, no exporter created.
		return &Provider{}, nil
	}

	svcName := os.Getenv("OTEL_SERVICE_NAME")
	if svcName == "" {
		svcName = DefaultServiceName
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(), // honors OTEL_RESOURCE_ATTRIBUTES
		resource.WithAttributes(semconv.ServiceName(svcName)),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	expCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	exp, err := otlptracegrpc.New(expCtx,
		otlptracegrpc.WithEndpoint(stripScheme(endpoint)),
		otlptracegrpc.WithInsecure(),
	)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("telemetry: trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp),
	)
	otel.SetTracerProvider(tp)

	p := &Provider{TracerProvider: tp}
	p.shutdown = append(p.shutdown, tp.Shutdown)
	return p, nil
}

// stripScheme normalizes endpoints — operators sometimes paste a URL
// ("http://host:4317") because they copied it; the gRPC exporter wants
// a bare "host:4317".
func stripScheme(s string) string {
	for _, p := range []string{"http://", "https://"} {
		if strings.HasPrefix(s, p) {
			return s[len(p):]
		}
	}
	return s
}
