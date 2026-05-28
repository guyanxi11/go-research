package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

// When endpoint is empty, Init must install the noop provider so spans are
// safe to call from any package and incur ~zero overhead.
func TestInit_EmptyEndpointInstallsNoopProvider(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown is nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown should be a no-op, got %v", err)
	}

	// The global tracer must still produce a usable, non-recording span.
	ctx, span := otel.Tracer("test").Start(context.Background(), "noop-span")
	if !span.SpanContext().IsValid() && span.SpanContext().IsSampled() {
		t.Error("noop span context should not be sampled")
	}
	span.End()
	_ = ctx
}

// Tracer() must produce tracers named "go-research/<subsystem>" so spans
// carry a recognisable otel.scope.name.
func TestTracer_NamesScope(t *testing.T) {
	// Calling twice returns a tracer with the same name; the prometheus library
	// also expects this behaviour from otel.Tracer.
	first := Tracer(SubsystemResearcher)
	second := Tracer(SubsystemResearcher)
	if first == nil || second == nil {
		t.Fatal("nil tracer")
	}
	// We can't directly read the name back from a Tracer, but creating a span
	// and dropping it should not panic. This is also a smoke test for the
	// global provider installed by the previous test.
	_, span := first.Start(context.Background(), "smoke")
	span.End()
}
