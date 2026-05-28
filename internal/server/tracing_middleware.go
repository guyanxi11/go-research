package server

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/yourname/go-research/internal/tracing"
)

// httpTracing produces one root span per HTTP request. The span is named
// "<METHOD> <route-template>" (low cardinality) and carries minimal HTTP
// semantic attributes so it shows up nicely in Jaeger/Tempo without leaking
// arbitrary user data.
func httpTracing() app.HandlerFunc {
	tracer := tracing.Tracer(tracing.SubsystemServer)
	return func(c context.Context, ctx *app.RequestContext) {
		// Extract any traceparent from inbound headers before starting our span.
		parent := otel.GetTextMapPropagator().Extract(c, propagation.HeaderCarrier(hertzHeaderToHTTP(ctx)))
		route := ctx.FullPath()
		if route == "" {
			route = string(ctx.URI().Path())
		}
		method := string(ctx.Method())
		spanName := method + " " + route
		spanCtx, span := tracer.Start(parent, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(method),
				attribute.String("http.route", route),
			),
		)
		defer span.End()

		ctx.Next(spanCtx)

		status := ctx.Response.StatusCode()
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= 500 {
			span.SetAttributes(attribute.String("error", strconv.Itoa(status)))
		}
	}
}
