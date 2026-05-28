package server

import (
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
)

// metricsResponseRecorder is a minimal http.ResponseWriter that buffers
// promhttp's exposition output so we can write it back through Hertz.
type metricsResponseRecorder struct {
	header      http.Header
	body        []byte
	contentType string
}

func (m *metricsResponseRecorder) Header() http.Header {
	if m.header == nil {
		m.header = http.Header{}
	}
	return m.header
}

func (m *metricsResponseRecorder) Write(p []byte) (int, error) {
	if m.contentType == "" {
		if ct := m.Header().Get("Content-Type"); ct != "" {
			m.contentType = ct
		} else {
			m.contentType = "text/plain; version=0.0.4; charset=utf-8"
		}
	}
	m.body = append(m.body, p...)
	return len(p), nil
}

func (m *metricsResponseRecorder) WriteHeader(int) {}

// requestForMetrics builds a net/http.Request stub for promhttp; promhttp
// only inspects Header and Method, neither of which we need to forward
// from the Hertz request (the endpoint is always GET /metrics).
func requestForMetrics(_ *app.RequestContext) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	return req
}
