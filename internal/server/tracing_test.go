package server

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// installRecorder swaps in a span recorder so we can assert on the exact spans
// produced by a handler. Restores the previous TracerProvider on test cleanup
// to keep tests independent.
func installRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	})
	return sr
}

func TestHTTPTracing_CreatesRootSpanPerRequest(t *testing.T) {
	sr := installRecorder(t)

	opt := config.NewOptions(nil)
	e := route.NewEngine(opt)
	e.Use(httpTracing())
	e.GET("/ok/:id", func(_ context.Context, c *app.RequestContext) {
		c.JSON(200, "ok")
	})

	ut.PerformRequest(e, "GET", "/ok/42", nil)
	ut.PerformRequest(e, "GET", "/ok/7", nil)

	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("recorded %d spans, want 2", len(spans))
	}
	for _, s := range spans {
		if got, want := s.Name(), "GET /ok/:id"; got != want {
			t.Errorf("span name = %q, want %q (route template, not concrete URL)", got, want)
		}
		var foundRoute, foundStatus bool
		for _, kv := range s.Attributes() {
			switch string(kv.Key) {
			case "http.route":
				foundRoute = true
				if kv.Value.AsString() != "/ok/:id" {
					t.Errorf("http.route = %q, want /ok/:id", kv.Value.AsString())
				}
			case "http.response.status_code":
				foundStatus = true
				if kv.Value.AsInt64() != 200 {
					t.Errorf("status = %d, want 200", kv.Value.AsInt64())
				}
			}
		}
		if !foundRoute || !foundStatus {
			t.Errorf("span missing required attrs (route=%v, status=%v)", foundRoute, foundStatus)
		}
	}
}

func TestHTTPTracing_5xxSetsErrorAttr(t *testing.T) {
	sr := installRecorder(t)

	opt := config.NewOptions(nil)
	e := route.NewEngine(opt)
	e.Use(httpTracing())
	e.GET("/boom", func(_ context.Context, c *app.RequestContext) {
		c.JSON(500, "boom")
	})

	ut.PerformRequest(e, "GET", "/boom", nil)

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	var has5xxError bool
	for _, kv := range spans[0].Attributes() {
		if string(kv.Key) == "error" && kv.Value.AsString() == "500" {
			has5xxError = true
		}
	}
	if !has5xxError {
		t.Error("5xx response should set error=500 attribute on the span")
	}
}
