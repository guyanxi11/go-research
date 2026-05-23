// Command tavily-smoke verifies TAVILY_API_KEY from .env with one live search call.
//
// Usage (from repo root):
//
//	go run ./cmd/tavily-smoke
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"github.com/yourname/go-research/internal/tool"
	"github.com/yourname/go-research/internal/tool/search"
)

func main() {
	_ = godotenv.Load()
	key := os.Getenv("TAVILY_API_KEY")
	if key == "" {
		log.Fatal("TAVILY_API_KEY is empty — add it to .env (https://app.tavily.com/)")
	}
	depth := os.Getenv("TAVILY_SEARCH_DEPTH")
	if depth == "" {
		depth = "basic"
	}

	tav := search.NewTavily(key, depth)
	args, _ := json.Marshal(tool.SearchArgs{
		Query:    "Go goroutine vs OS thread memory",
		MaxItems: 3,
	})
	raw, err := tav.Call(context.Background(), args)
	if err != nil {
		log.Fatalf("tavily search failed: %v", err)
	}
	var out tool.SearchResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		log.Fatalf("decode: %v", err)
	}
	fmt.Printf("OK: %d results (depth=%s)\n", len(out.Items), depth)
	for i, it := range out.Items {
		fmt.Printf("  [%d] %s\n      %s\n", i+1, it.Title, it.URL)
	}
}
