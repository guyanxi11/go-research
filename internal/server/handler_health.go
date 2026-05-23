package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (s *Server) handleHealthz(ctx context.Context, c *app.RequestContext) {
	searchMode := "mock"
	if s.cfg.TavilyAPIKey != "" {
		searchMode = "tavily"
	}
	out := utils.H{
		"status":       "ok",
		"model":        s.llm.ModelName(),
		"search":       searchMode,
		"persistence":  "disabled",
	}
	if s.db != nil {
		if err := s.db.Ping(ctx); err != nil {
			c.JSON(consts.StatusServiceUnavailable, utils.H{
				"status":      "degraded",
				"model":       s.llm.ModelName(),
				"persistence": "postgres",
				"db_error":    err.Error(),
			})
			return
		}
		out["persistence"] = "postgres"
	}
	c.JSON(consts.StatusOK, out)
}
