package server

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/yourname/go-research/internal/store"
)

// handleResearchList returns persisted research sessions (newest first).
// GET /api/research?limit=20&offset=0
func (s *Server) handleResearchList(ctx context.Context, c *app.RequestContext) {
	if s.store == nil {
		c.JSON(consts.StatusServiceUnavailable, utils.H{
			"error": "persistence disabled (postgres not connected)",
		})
		return
	}
	limit, _ := strconv.Atoi(string(c.Query("limit")))
	offset, _ := strconv.Atoi(string(c.Query("offset")))

	items, err := s.store.ListSessions(ctx, limit, offset)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []store.SessionSummary{}
	}
	c.JSON(consts.StatusOK, utils.H{"items": items})
}

// handleResearchGet returns one session with tasks and report.
// GET /api/research/:id
func (s *Server) handleResearchGet(ctx context.Context, c *app.RequestContext) {
	if s.store == nil {
		c.JSON(consts.StatusServiceUnavailable, utils.H{
			"error": "persistence disabled (postgres not connected)",
		})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "id is required"})
		return
	}
	det, err := s.store.GetSession(ctx, id)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	if det == nil {
		c.JSON(consts.StatusNotFound, utils.H{"error": "session not found"})
		return
	}
	c.JSON(consts.StatusOK, det)
}
