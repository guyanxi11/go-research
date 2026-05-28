// Package llm wraps the underlying ChatModel implementation so the rest of the
// codebase (agents, tools, server handlers) only talks to a small, stable
// interface. Today we use Eino's OpenAI-compatible ChatModel; swapping it for
// another provider later is a one-file change.
package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/yourname/go-research/internal/config"
	"github.com/yourname/go-research/internal/metrics"
)

// Client is the narrow surface the rest of the app depends on. Keeping it
// minimal makes it trivial to mock in tests and to add a second backend later.
type Client struct {
	model model.ChatModel
	name  string
}

func New(ctx context.Context, cfg config.LLMConfig) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm: api key is empty")
	}
	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("llm: new openai chat model: %w", err)
	}
	return &Client{model: cm, name: cfg.Model}, nil
}

func (c *Client) ModelName() string { return c.name }

// Stream sends the conversation and returns a reader that yields incremental
// schema.Message deltas. Caller is responsible for closing the reader.
//
// We record latency at call-establishment time (i.e. when the upstream
// accepts the request and returns a reader), NOT until the stream is fully
// drained — those two timings answer different questions and the stream
// duration belongs to the caller.
func (c *Client) Stream(ctx context.Context, msgs []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	start := time.Now()
	r, err := c.model.Stream(ctx, msgs)
	outcome := metrics.Outcome(err)
	metrics.LLMRequestsTotal.WithLabelValues("stream", outcome).Inc()
	metrics.LLMRequestDurationSeconds.WithLabelValues("stream", outcome).Observe(time.Since(start).Seconds())
	return r, err
}

// Generate is the non-streaming variant, useful for short utility prompts
// (e.g. classification, summarisation) where streaming buys nothing.
func (c *Client) Generate(ctx context.Context, msgs []*schema.Message) (*schema.Message, error) {
	start := time.Now()
	out, err := c.model.Generate(ctx, msgs)
	outcome := metrics.Outcome(err)
	metrics.LLMRequestsTotal.WithLabelValues("generate", outcome).Inc()
	metrics.LLMRequestDurationSeconds.WithLabelValues("generate", outcome).Observe(time.Since(start).Seconds())
	if err == nil && out != nil {
		metrics.LLMOutputCharsTotal.WithLabelValues("generate").Add(float64(len(out.Content)))
	}
	return out, err
}
