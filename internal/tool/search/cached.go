package search

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yourname/go-research/internal/tool"
)

// SearchCache memoizes search tool JSON responses.
type SearchCache interface {
	GetSearch(ctx context.Context, query string) (string, bool, error)
	SetSearch(ctx context.Context, query, jsonResult string) error
}

// Cached wraps an inner search Tool with Redis (or any SearchCache) backing.
type Cached struct {
	inner tool.Tool
	cache SearchCache
}

func NewCached(inner tool.Tool, cache SearchCache) *Cached {
	return &Cached{inner: inner, cache: cache}
}

func (c *Cached) Name() string        { return c.inner.Name() }
func (c *Cached) Description() string { return c.inner.Description() }

func (c *Cached) Call(ctx context.Context, argsJSON json.RawMessage) (string, error) {
	var args tool.SearchArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", fmt.Errorf("cached search: decode args: %w", err)
	}
	query := args.Query
	if query == "" {
		return c.inner.Call(ctx, argsJSON)
	}

	if hit, ok, err := c.cache.GetSearch(ctx, query); err == nil && ok {
		return hit, nil
	}

	out, err := c.inner.Call(ctx, argsJSON)
	if err != nil {
		return "", err
	}
	_ = c.cache.SetSearch(ctx, query, out)
	return out, nil
}
