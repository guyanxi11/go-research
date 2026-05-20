package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (s *Server) routes() {
	s.h.GET("/healthz", func(_ context.Context, c *app.RequestContext) {
		c.JSON(consts.StatusOK, utils.H{
			"status": "ok",
			"model":  s.llm.ModelName(),
		})
	})

	s.h.StaticFile("/", "./web/index.html")
	s.h.Static("/static", "./web")

	api := s.h.Group("/api")
	api.POST("/chat", s.handleChat)
	api.POST("/research", s.handleResearch)
}
