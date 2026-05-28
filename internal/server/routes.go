package server

import hlog "github.com/cloudwego/hertz/pkg/common/hlog"

func (s *Server) routes() {
	// Health and static assets stay public so probes / browser visits keep
	// working even when API_KEY is enabled.
	s.h.GET("/healthz", s.handleHealthz)
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
