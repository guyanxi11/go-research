package server

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yourname/go-research/internal/metrics"
)

// httpMetrics is a Hertz middleware that records request counters, in-flight
// gauge and latency histogram with low-cardinality labels.
//
// The route label uses the Hertz-matched route template (e.g. /api/research/:id)
// rather than the concrete URL to keep label cardinality bounded.
func httpMetrics() app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		metrics.HTTPInFlight.Inc()
		start := time.Now()

		ctx.Next(c)

		metrics.HTTPInFlight.Dec()
		route := ctx.FullPath()
		if route == "" {
			route = string(ctx.URI().Path())
		}
		method := string(ctx.Method())
		statusClass := metrics.StatusClass(ctx.Response.StatusCode())
		elapsed := time.Since(start).Seconds()
		metrics.HTTPRequestsTotal.WithLabelValues(route, method, statusClass).Inc()
		metrics.HTTPRequestDurationSeconds.WithLabelValues(route, method, statusClass).Observe(elapsed)
	}
}
