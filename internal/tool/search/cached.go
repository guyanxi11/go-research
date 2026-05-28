package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yourname/go-research/internal/metrics"
	"github.com/yourname/go-research/internal/tool"
)

// cacheKeyVersion is bumped whenever the cached payload schema changes so old
// entries are naturally invalidated instead of requiring a manual flush.
const cacheKeyVersion = "v2"

// SearchCache memoizes search tool JSON responses by an opaque key.
//
// The key is composed by the caller and may already be hashed; backends
// MUST treat it as opaque and only add their own namespace prefix.
type SearchCache interface {
	GetSearch(ctx context.Context, key string) (string, bool, error)
	SetSearch(ctx context.Context, key, jsonResult string) error
}

// Cached wraps an inner search Tool with Redis (or any SearchCache) backing.
//
// The cache key is derived from the provider namespace, the canonical args
// (query + max_items + ...) and a schema version, so changing depth/provider
// or evolving the cached payload format never returns stale hits.
type Cached struct {
	inner     tool.Tool
	cache     SearchCache
	namespace string
}

// NewCached creates a cache wrapper around an existing search tool.
//
// namespace MUST uniquely identify the provider configuration that affects
// results (e.g. "tavily:advanced", "tavily:basic", "mock"). Two providers
// with different namespaces never share cache entries.
func NewCached(inner tool.Tool, cache SearchCache, namespace string) *Cached {
	if namespace == "" {
		namespace = inner.Name()
	}
	return &Cached{inner: inner, cache: cache, namespace: namespace}
}

func (c *Cached) Name() string        { return c.inner.Name() }
func (c *Cached) Description() string { return c.inner.Description() }

func (c *Cached) Call(ctx context.Context, argsJSON json.RawMessage) (string, error) {
	start := time.Now()

	var args tool.SearchArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", fmt.Errorf("cached search: decode args: %w", err)
	}
	if args.Query == "" {
		out, err := c.inner.Call(ctx, argsJSON)
		c.recordMetric("miss", start, err)
		return out, err
	}

	key := c.buildKey(args)
	if hit, ok, err := c.cache.GetSearch(ctx, key); err == nil && ok {
		c.recordMetric("hit", start, nil)
		return hit, nil
	}

	out, err := c.inner.Call(ctx, argsJSON)
	c.recordMetric("miss", start, err)
	if err != nil {
		return "", err
	}
	_ = c.cache.SetSearch(ctx, key, out)
	return out, nil
}

func (c *Cached) recordMetric(cacheLabel string, start time.Time, err error) {
	// cache="hit" wins over outcome — a hit can't fail. For miss we still
	// record latency including the upstream provider call.
	provider := c.namespace
	metrics.SearchRequestsTotal.WithLabelValues(provider, cacheLabel).Inc()
	metrics.SearchRequestDurationSeconds.WithLabelValues(provider, cacheLabel).
		Observe(time.Since(start).Seconds())
	_ = err // err is already reflected by upstream's own metrics / logs
}

// buildKey produces a stable opaque cache key.
func (c *Cached) buildKey(args tool.SearchArgs) string {
	canonical, _ := json.Marshal(struct {
		V  string `json:"v"`
		NS string `json:"ns"`
		Q  string `json:"q"`
		M  int    `json:"m"`
	}{cacheKeyVersion, c.namespace, args.Query, args.MaxItems})
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
