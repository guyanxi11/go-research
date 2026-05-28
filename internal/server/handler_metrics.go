package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// handleMetrics serves Prometheus exposition format on /metrics.
//
// Hertz handlers cannot reuse net/http handlers directly, so we collect the
// metrics via the gatherer and write them with Hertz's writer. The default
// gatherer covers every collector registered via promauto (which is what
// our internal/metrics package uses).
func handleMetrics() app.HandlerFunc {
	gatherer := prometheus.DefaultGatherer
	opts := promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
		Registry:      prometheus.DefaultRegisterer,
	}
	h := promhttp.HandlerFor(gatherer, opts)
	return func(c context.Context, ctx *app.RequestContext) {
		// Adapt Hertz's RequestContext to net/http. Hertz ships a helper for
		// this, but the simplest portable approach is to let promhttp write
		// to a recorder and copy the body across.
		rec := &metricsResponseRecorder{}
		h.ServeHTTP(rec, requestForMetrics(ctx))
		ctx.SetStatusCode(consts.StatusOK)
		ctx.Response.Header.SetContentType(rec.contentType)
		_, _ = ctx.Write(rec.body)
	}
}
