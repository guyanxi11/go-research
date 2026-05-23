package main

import (
	"context"
	"log"
	"time"

	hlog "github.com/cloudwego/hertz/pkg/common/hlog"

	"github.com/yourname/go-research/internal/agent/dag"
	"github.com/yourname/go-research/internal/agent/orchestrator"
	"github.com/yourname/go-research/internal/agent/researcher"
	rediscache "github.com/yourname/go-research/internal/cache/redis"
	"github.com/yourname/go-research/internal/config"
	"github.com/yourname/go-research/internal/llm"
	"github.com/yourname/go-research/internal/server"
	"github.com/yourname/go-research/internal/store/postgres"
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

	pgStore, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		log.Fatalf("postgres: %v\n(hint: start Postgres with `make up` in the project root)", err)
	}
	defer pgStore.Close()
	hlog.Infof("postgres ready: %s:%d/%s", cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DB)

	var searchCache search.SearchCache
	rc := rediscache.Open(cfg.Redis)
	if err := rc.Ping(ctx); err != nil {
		hlog.Warnf("redis unavailable, search cache disabled: %v", err)
		_ = rc.Close()
	} else {
		searchCache = rc
		defer rc.Close()
		hlog.Infof("redis ready: %s (search TTL %s)", cfg.Redis.Addr, "1h")
	}

	tools := tool.NewRegistry()
	var searchTool tool.Tool
	if cfg.TavilyAPIKey != "" {
		searchTool = search.NewTavily(cfg.TavilyAPIKey, cfg.TavilySearchDepth)
		hlog.Infof("search tool: tavily (depth=%s)", cfg.TavilySearchDepth)
	} else {
		searchTool = search.NewMock()
		hlog.Warnf("search tool: MOCK (set TAVILY_API_KEY in .env for real search)")
	}
	if searchCache != nil {
		searchTool = search.NewCached(searchTool, searchCache)
	}
	if err := tools.Register(searchTool); err != nil {
		log.Fatalf("register search: %v", err)
	}

	scheduler := dag.NewScheduler(
		dag.WithConcurrency(6),
		dag.WithBackoff(dag.ExponentialBackoff(200*time.Millisecond, 3*time.Second)),
	)
	researchOpts := researcher.Options{
		MaxSearchRounds:    cfg.Agent.MaxSearchRounds,
		MaxFollowUpQueries: cfg.Agent.MaxFollowUpQueries,
		CriticEnabled:      cfg.Agent.CriticEnabled,
		CriticMinScore:     cfg.Agent.CriticMinScore,
		MaxCriticRetries:   cfg.Agent.MaxCriticRetries,
	}
	hlog.Infof("agent: search_rounds=%d critic=%v min_score=%d",
		researchOpts.MaxSearchRounds, researchOpts.CriticEnabled, researchOpts.CriticMinScore)

	orch := orchestrator.New(orchestrator.Deps{
		LLM:            llmClient,
		Tools:          tools,
		Scheduler:      scheduler,
		ResearcherOpts: researchOpts,
	})

	if err := server.New(cfg, llmClient, orch, pgStore, pgStore).Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
