// Package metrics owns every Prometheus collector the service exposes.
//
// All collectors are package-level vars registered against the default
// prometheus.Registerer at process start, so multiple instantiations within
// one process (e.g. tests) MUST go through prometheus.MustRegister via
// promauto rather than re-creating collectors.
//
// Naming follows the Prometheus best-practice:
//   - _total      suffix for counters
//   - _seconds    suffix for time-valued histograms (NOT _ms, see official docs)
//   - _bytes      suffix for size-valued histograms
//
// All labels are low-cardinality (status code class, agent name, provider,
// cache hit/miss). Never label by user id / question / URL.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// latencyBuckets covers 1ms..30s, biased towards the LLM-call regime where
// 100ms..15s is where most signal lives.
var latencyBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 15, 30,
}

// ===== HTTP =====

// HTTPRequestsTotal counts HTTP requests by route template, method and the
// status code class ("2xx", "4xx", "5xx" — concrete codes blow up cardinality
// without adding much signal).
var HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "http_requests_total",
	Help: "Total HTTP requests handled, partitioned by route template, method and status class.",
}, []string{"route", "method", "status"})

// HTTPRequestDurationSeconds tracks end-to-end handler latency.
var HTTPRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "HTTP handler latency in seconds.",
	Buckets: latencyBuckets,
}, []string{"route", "method", "status"})

// HTTPInFlight is a gauge for currently-being-served requests; useful for
// detecting slow-loris-style pile-ups and Hertz worker saturation.
var HTTPInFlight = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "http_in_flight_requests",
	Help: "Number of HTTP requests currently being handled.",
})

// ===== LLM =====

// LLMRequestsTotal counts every Generate / Stream call to the upstream LLM.
var LLMRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "llm_requests_total",
	Help: "Total upstream LLM calls, partitioned by mode (generate/stream) and outcome (ok/error).",
}, []string{"mode", "outcome"})

// LLMRequestDurationSeconds tracks upstream LLM latency.
var LLMRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "llm_request_duration_seconds",
	Help:    "Upstream LLM call latency in seconds.",
	Buckets: latencyBuckets,
}, []string{"mode", "outcome"})

// LLMOutputCharsTotal proxies for output tokens since we don't have a real
// tokenizer in the loop (cheap and good enough for dashboards).
var LLMOutputCharsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "llm_output_chars_total",
	Help: "Total characters returned by the upstream LLM (proxy for output tokens).",
}, []string{"mode"})

// ===== Search tool =====

// SearchRequestsTotal counts every search.Call, partitioned by provider
// namespace ("tavily:basic" / "mock") and cache outcome ("hit" / "miss").
var SearchRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "search_requests_total",
	Help: "Total search-tool calls, partitioned by provider namespace and cache outcome.",
}, []string{"provider", "cache"})

// SearchRequestDurationSeconds tracks search-tool call latency.
var SearchRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "search_request_duration_seconds",
	Help:    "Search-tool call latency in seconds.",
	Buckets: latencyBuckets,
}, []string{"provider", "cache"})

// ===== Agent / DAG =====

// AgentStepDurationSeconds tracks per-agent end-to-end time. "agent" is one
// of planner / researcher / critic / writer.
var AgentStepDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "agent_step_duration_seconds",
	Help:    "Per-agent step duration in seconds.",
	Buckets: latencyBuckets,
}, []string{"agent", "outcome"})

// DAGNodesInFlight is the live count of DAG worker goroutines currently
// executing a node.
var DAGNodesInFlight = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "dag_nodes_in_flight",
	Help: "Number of DAG nodes currently executing.",
})

// DAGNodesTotal counts DAG node terminations by outcome.
var DAGNodesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "dag_nodes_total",
	Help: "Total DAG nodes that finished, partitioned by outcome (ok/failed/canceled).",
}, []string{"outcome"})

// ===== Research session =====

// ResearchSessionsTotal counts research pipeline runs by terminal status.
var ResearchSessionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "research_sessions_total",
	Help: "Total /api/research pipelines completed, partitioned by status (done/failed/canceled).",
}, []string{"status"})

// ResearchSessionDurationSeconds tracks total pipeline wall-clock time.
var ResearchSessionDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "research_session_duration_seconds",
	Help:    "End-to-end /api/research pipeline duration in seconds.",
	Buckets: []float64{1, 2, 5, 10, 15, 30, 60, 120, 180},
}, []string{"status"})

// ResearchReportCharsTotal accumulates report sizes; divide by sessions to
// get average chars/report on the dashboard.
var ResearchReportCharsTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "research_report_chars_total",
	Help: "Total characters in completed research reports.",
})

// StatusClass collapses HTTP status codes into 2xx/3xx/4xx/5xx labels.
func StatusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

// Outcome maps an error to a low-cardinality label.
func Outcome(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}
