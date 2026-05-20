package main

import (
	"context"
	"log"

	hlog "github.com/cloudwego/hertz/pkg/common/hlog"

	"github.com/yourname/go-research/internal/config"
	"github.com/yourname/go-research/internal/llm"
	"github.com/yourname/go-research/internal/server"
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

	if err := server.New(cfg, llmClient).Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
