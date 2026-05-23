package server

func (s *Server) routes() {
	s.h.GET("/healthz", s.handleHealthz)

	s.h.StaticFile("/", "./web/index.html")
	s.h.Static("/static", "./web")

	api := s.h.Group("/api")
	api.POST("/chat", s.handleChat)
	api.POST("/research", s.handleResearch)
	api.GET("/research", s.handleResearchList)
	api.GET("/research/:id", s.handleResearchGet)
}
