package server

import hlog "github.com/cloudwego/hertz/pkg/common/hlog"

func (s *Server) routes() {
	// Tracing is outermost so the root span covers metrics middleware AND the
	// handler. Every downstream agent/llm/search span will naturally parent to
	// this one via context.Context. /metrics and /healthz produce noise-but-
	// cheap spans; if that ever shows up in Jaeger as clutter, gate them here.
	s.h.Use(httpTracing())

	// HTTP request metrics are recorded for every route, including /healthz
	// and /metrics, so dashboards reflect real traffic shape (including
	// probes). Cardinality is bounded by FullPath() route templates.
	s.h.Use(httpMetrics())

	// Health, metrics and static assets stay public so probes / browser
	// visits / Prometheus scrapes keep working even when API_KEY is enabled.
	s.h.GET("/healthz", s.handleHealthz)
	s.h.GET("/metrics", handleMetrics())
	s.h.StaticFile("/", "./web/index.html")
	s.h.Static("/static", "./web")

	api := s.h.Group("/api")
	if s.cfg.APIKey != "" {
		api.Use(requireAPIKey(s.cfg.APIKey))
		hlog.Infof("API auth: X-API-Key required for /api/*")
	} else {
		hlog.Warnf("API auth: DISABLED (set API_KEY in .env to require X-API-Key)")
	}
	api.POST("/chat", s.handleChat)
	api.POST("/research", s.handleResearch)
	api.GET("/research", s.handleResearchList)
	api.GET("/research/:id", s.handleResearchGet)
}
