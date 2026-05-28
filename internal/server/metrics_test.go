package server

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/yourname/go-research/internal/metrics"
)

// newTestEngine builds a Hertz route.Engine with our metrics middleware
// installed so we can hit it via ut.PerformRequest.
func newTestEngine(t *testing.T, routes func(e *route.Engine)) *route.Engine {
	t.Helper()
	opt := config.NewOptions(nil)
	e := route.NewEngine(opt)
	e.Use(httpMetrics())
	routes(e)
	return e
}

func TestHandleMetrics_ExposesPromFormat(t *testing.T) {
	e := newTestEngine(t, func(e *route.Engine) {
		e.GET("/dummy", func(_ context.Context, c *app.RequestContext) {
			c.JSON(200, "ok")
		})
		e.GET("/metrics", handleMetrics())
	})

	// Prometheus *Vec collectors only emit lines for materialised series.
	// Trigger one HTTP request and one synthetic increment of each label
	// group we care about so the exposition contains every collector.
	ut.PerformRequest(e, "GET", "/dummy", nil)
	metrics.LLMRequestsTotal.WithLabelValues("generate", "ok").Inc()
	metrics.SearchRequestsTotal.WithLabelValues("mock", "hit").Inc()
	metrics.DAGNodesTotal.WithLabelValues("ok").Inc()
	metrics.ResearchSessionsTotal.WithLabelValues("done").Inc()

	w := ut.PerformRequest(e, "GET", "/metrics", nil)
	rsp := w.Result()
	if got := rsp.StatusCode(); got != 200 {
		t.Fatalf("status = %d, want 200", got)
	}
	body := string(rsp.Body())
	for _, want := range []string{
		// always-present (single metrics)
		"http_in_flight_requests",
		"dag_nodes_in_flight",
		"research_report_chars_total",
		// materialised vec metrics
		"http_requests_total",
		"http_request_duration_seconds",
		"llm_requests_total",
		"search_requests_total",
		"dag_nodes_total",
		"research_sessions_total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics body is missing %q", want)
		}
	}
}

func TestHTTPMetrics_CountsRequestsByRouteAndClass(t *testing.T) {
	e := newTestEngine(t, func(e *route.Engine) {
		e.GET("/ok/:id", func(_ context.Context, c *app.RequestContext) {
			c.JSON(200, "ok")
		})
		e.GET("/boom", func(_ context.Context, c *app.RequestContext) {
			c.JSON(500, "boom")
		})
	})

	// Snapshot before
	beforeOK := readCounter(t, "http_requests_total",
		map[string]string{"route": "/ok/:id", "method": "GET", "status": "2xx"})
	beforeErr := readCounter(t, "http_requests_total",
		map[string]string{"route": "/boom", "method": "GET", "status": "5xx"})

	// Same template, two different concrete URLs — must collapse into one series.
	ut.PerformRequest(e, "GET", "/ok/1", nil)
	ut.PerformRequest(e, "GET", "/ok/2", nil)
	ut.PerformRequest(e, "GET", "/boom", nil)

	gotOK := readCounter(t, "http_requests_total",
		map[string]string{"route": "/ok/:id", "method": "GET", "status": "2xx"})
	gotErr := readCounter(t, "http_requests_total",
		map[string]string{"route": "/boom", "method": "GET", "status": "5xx"})

	if delta := gotOK - beforeOK; delta != 2 {
		t.Errorf("/ok/:id 2xx counter delta = %v, want 2", delta)
	}
	if delta := gotErr - beforeErr; delta != 1 {
		t.Errorf("/boom 5xx counter delta = %v, want 1", delta)
	}
}

// readCounter pulls the current value of a labelled CounterVec series.
func readCounter(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	if name != "http_requests_total" {
		t.Fatalf("readCounter helper only knows http_requests_total; got %q", name)
	}
	c, err := metrics.HTTPRequestsTotal.GetMetricWith(labels)
	if err != nil {
		t.Fatalf("GetMetricWith: %v", err)
	}
	return readCounterValue(c)
}
