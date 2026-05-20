package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app"
	hlog "github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
)

type chatRequest struct {
	Message string                  `json:"message"`
	History []chatRequestHistoryMsg `json:"history,omitempty"`
}

type chatRequestHistoryMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const defaultSystemPrompt = `You are GoResearch, a research assistant. Answer concisely and ` +
	`cite sources when possible. Phase 1 build: no external tools wired yet — rely on your own knowledge.`

// handleChat is the Phase 1 chat endpoint. It only does single-turn ReAct-less
// streaming: client sends a message, server streams the LLM tokens back via SSE.
// Multi-agent orchestration arrives in Phase 2.
func (s *Server) handleChat(ctx context.Context, c *app.RequestContext) {
	var req chatRequest
	if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.Message == "" {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	msgs := buildMessages(req)

	stream, err := s.llm.Stream(ctx, msgs)
	if err != nil {
		hlog.Errorf("llm stream init: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "llm unavailable"})
		return
	}
	defer stream.Close()

	w := sse.NewWriter(c)
	defer w.Close()

	var (
		eventID int
		total   int
	)
	emit := func(eventType, data string) error {
		eventID++
		return w.WriteEvent(strconv.Itoa(eventID), eventType, []byte(data))
	}

	if err := emit("start", `{"model":"`+s.llm.ModelName()+`"}`); err != nil {
		hlog.Errorf("sse start: %v", err)
		return
	}

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			hlog.Errorf("llm stream recv: %v", err)
			_ = emit("error", jsonMust(map[string]string{"error": err.Error()}))
			return
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}
		total += len(chunk.Content)
		if err := emit("token", jsonMust(map[string]string{"delta": chunk.Content})); err != nil {
			hlog.Errorf("sse token: %v", err)
			return
		}
	}

	_ = emit("done", jsonMust(map[string]int{"chars": total}))
}

func buildMessages(req chatRequest) []*schema.Message {
	msgs := make([]*schema.Message, 0, len(req.History)+2)
	msgs = append(msgs, schema.SystemMessage(defaultSystemPrompt))
	for _, h := range req.History {
		switch h.Role {
		case "user":
			msgs = append(msgs, schema.UserMessage(h.Content))
		case "assistant":
			msgs = append(msgs, schema.AssistantMessage(h.Content, nil))
		}
	}
	msgs = append(msgs, schema.UserMessage(req.Message))
	return msgs
}

func jsonMust(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{}`
	}
	return string(b)
}
