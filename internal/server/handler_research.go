package server

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	hlog "github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	"github.com/google/uuid"

	"github.com/yourname/go-research/internal/agent/orchestrator"
)

type researchRequest struct {
	Question string `json:"question"`
}

// handleResearch is the Phase 2.B end-to-end pipeline endpoint:
// Planner -> DAG of Researchers -> Writer, all of it streamed back as SSE.
//
// Event types (see orchestrator.EventType):
//   - plan          : the Planner's full subtask DAG
//   - node_started  : a Researcher began work on a subtask
//   - node_finished : a Researcher returned findings + citations
//   - node_failed   : a Researcher exhausted retries
//   - writer_token  : a Writer-emitted token, ready to render
//   - session       : persisted run id (Phase 3, only when Postgres is up)
//   - done          : pipeline complete (elapsed_ms, chars)
//   - error         : pipeline aborted (stage + message)
func (s *Server) handleResearch(ctx context.Context, c *app.RequestContext) {
	var req researchRequest
	if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.Question == "" {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "question is required"})
		return
	}

	w := sse.NewWriter(c)
	defer w.Close()

	var sessionID string
	var recorder *researchRecorder
	if s.store != nil {
		sessionID = uuid.NewString()
		if err := s.store.CreateSession(ctx, sessionID, req.Question); err != nil {
			hlog.Errorf("create research session: %v", err)
			c.JSON(consts.StatusInternalServerError, map[string]string{"error": "failed to create session"})
			return
		}
		recorder = newResearchRecorder(s.store, sessionID)
		payload, _ := json.Marshal(map[string]string{"id": sessionID})
		if err := w.WriteEvent("0", "session", payload); err != nil {
			hlog.Warnf("research SSE session event: %v", err)
		}
	}

	// Generously buffered so the orchestrator never blocks on a slow client
	// while still preserving full-throughput when the consumer is fast.
	events := make(chan orchestrator.Event, 1024)

	// Pipeline runs in its own goroutine so the HTTP handler can fan events
	// out to SSE without buffering them in memory first.
	//
	// A total deadline guards against hung LLM calls / runaway ReAct loops so
	// the request never leaks goroutines forever. Disabled when configured
	// to <= 0 (not recommended outside local debugging).
	pipelineErr := make(chan error, 1)
	var (
		pipelineCtx context.Context
		cancel      context.CancelFunc
	)
	if s.cfg.ResearchTimeoutSeconds > 0 {
		pipelineCtx, cancel = context.WithTimeout(ctx,
			time.Duration(s.cfg.ResearchTimeoutSeconds)*time.Second)
	} else {
		pipelineCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	go func() {
		defer close(events)
		pipelineErr <- s.orch.Run(pipelineCtx, req.Question, events)
	}()

	var eventID int
	for ev := range events {
		eventID++
		if recorder != nil {
			recorder.OnEvent(pipelineCtx, ev)
		}
		payload, _ := json.Marshal(ev.Payload)
		if err := w.WriteEvent(strconv.Itoa(eventID), string(ev.Type), payload); err != nil {
			// Client disconnected. Stop pulling, propagate cancellation to
			// the pipeline so in-flight LLM/search calls wind down.
			hlog.Warnf("research SSE write failed (client gone?): %v", err)
			cancel()
			// Drain events so the orchestrator goroutine doesn't leak.
			go func() {
				for range events {
				}
			}()
			break
		}
	}
	pErr := <-pipelineErr
	if recorder != nil {
		recorder.Finalize(ctx, pErr)
	}
	if pErr != nil {
		hlog.Errorf("research pipeline error: %v", pErr)
	}
	_ = sessionID
}
