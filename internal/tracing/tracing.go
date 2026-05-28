// Package tracing wires the OpenTelemetry SDK and exposes a single Init/Shutdown
// pair plus per-subsystem tracers.
//
// Design notes
//
//   - If the OTLP endpoint is empty, Init installs the global noop provider and
//     returns a no-op shutdown. All call sites that grab a tracer therefore
//     stay zero-overhead in local dev / CI when tracing is not configured.
//   - We expose otel.Tracer wrappers per subsystem (server, orchestrator,
//     planner, researcher, critic, writer, llm, search, dag). Each tracer name
//     becomes the `otel.scope.name` attribute on every span it produces.
//   - The exporter speaks OTLP/HTTP because Jaeger all-in-one accepts it on
//     :4318 without any extra collector, keeping docker-compose minimal.
//   - W3C TraceContext + Baggage are installed as the global propagator so
//     incoming traceparent headers (e.g. from a curl -H "traceparent: ...")
//     stitch into the produced trace.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config configures the tracing pipeline. Both fields are typically read from
// environment variables in cmd/server/main.go.
type Config struct {
	// Endpoint is the OTLP/HTTP collector endpoint, e.g. "localhost:4318".
	// Empty disables real tracing (installs the global noop provider).
	Endpoint string

	// ServiceName is the value of service.name in span resource attrs.
	ServiceName string

	// Insecure forces http:// (no TLS); required for local Jaeger.
	Insecure bool
}

// ShutdownFn is what Init returns: call it once at process exit to flush
// in-flight spans. It is always safe to call (no-op shutdown is also returned
// when tracing is disabled).
type ShutdownFn func(context.Context) error

// Init installs the global TracerProvider and Propagator. When cfg.Endpoint is
// empty, a noop provider is used so the rest of the codebase keeps zero
// overhead.
func Init(ctx context.Context, cfg Config) (ShutdownFn, error) {
	// Always set up the W3C propagator so incoming traceparent headers are
	// honoured even when our own exporter is off.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if cfg.Endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "go-research"
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("tracing: build otlp exporter: %w", err)
	}

	res, rerr := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if rerr != nil {
		_ = exp.Shutdown(ctx)
		return nil, fmt.Errorf("tracing: build resource: %w", rerr)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp,
			sdktrace.WithMaxQueueSize(2048),
			sdktrace.WithBatchTimeout(2*time.Second),
		),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return func(ctx context.Context) error {
		// Shutdown flushes pending batches. Bound it so a hung collector
		// can't block the process exit forever.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return errors.Join(tp.Shutdown(ctx), exp.Shutdown(ctx))
	}, nil
}

// Tracer returns an otel.Tracer named "go-research/<subsystem>". Use the
// constants below for the standard subsystems so every span carries a
// recognisable scope.name attribute.
func Tracer(subsystem string) trace.Tracer {
	return otel.Tracer("go-research/" + subsystem)
}

// Subsystem names used as otel.scope.name values throughout the codebase.
const (
	SubsystemServer       = "server"
	SubsystemOrchestrator = "orchestrator"
	SubsystemPlanner      = "planner"
	SubsystemResearcher   = "researcher"
	SubsystemCritic       = "critic"
	SubsystemWriter       = "writer"
	SubsystemDAG          = "dag"
	SubsystemLLM          = "llm"
	SubsystemSearch       = "search"
)
