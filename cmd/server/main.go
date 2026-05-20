package main

import (
	"context"
	"log"
	"time"

	hlog "github.com/cloudwego/hertz/pkg/common/hlog"

	"github.com/yourname/go-research/internal/agent/dag"
	"github.com/yourname/go-research/internal/agent/orchestrator"
	"github.com/yourname/go-research/internal/config"
	"github.com/yourname/go-research/internal/llm"
	"github.com/yourname/go-research/internal/server"
	"github.com/yourname/go-research/internal/tool"
	"github.com/yourname/go-research/internal/tool/search"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	llmClient, err := llm.New(ctx, cfg.LLM)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	hlog.Infof("LLM ready: model=%s base=%s", cfg.LLM.Model, cfg.LLM.BaseURL)

	tools := tool.NewRegistry()
	if cfg.TavilyAPIKey != "" {
		if err := tools.Register(search.NewTavily(cfg.TavilyAPIKey)); err != nil {
			log.Fatalf("register tavily: %v", err)
		}
		hlog.Infof("search tool: tavily")
	} else {
		if err := tools.Register(search.NewMock()); err != nil {
			log.Fatalf("register mock search: %v", err)
		}
		hlog.Warnf("search tool: MOCK (set TAVILY_API_KEY in .env for real search)")
	}

	scheduler := dag.NewScheduler(
		dag.WithConcurrency(6),
		dag.WithBackoff(dag.ExponentialBackoff(200*time.Millisecond, 3*time.Second)),
	)
	orch := orchestrator.New(orchestrator.Deps{
		LLM:       llmClient,
		Tools:     tools,
		Scheduler: scheduler,
	})

	if err := server.New(cfg, llmClient, orch).Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
