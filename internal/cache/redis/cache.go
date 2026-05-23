// Package redis provides optional Redis-backed caching for tool calls.
package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/yourname/go-research/internal/config"
)

const defaultSearchTTL = time.Hour

// Cache wraps a Redis client for search result memoization.
type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

// Open connects to Redis. Caller should Ping before relying on the cache.
func Open(cfg config.RedisConfig) *Cache {
	return &Cache{
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
		ttl: defaultSearchTTL,
	}
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

// GetSearch returns cached JSON for a search query, or ("", false, nil).
func (c *Cache) GetSearch(ctx context.Context, query string) (string, bool, error) {
	val, err := c.client.Get(ctx, searchKey(query)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// SetSearch stores search tool output JSON.
func (c *Cache) SetSearch(ctx context.Context, query, jsonResult string) error {
	return c.client.Set(ctx, searchKey(query), jsonResult, c.ttl).Err()
}

func searchKey(query string) string {
	sum := sha256.Sum256([]byte(query))
	return "search:" + hex.EncodeToString(sum[:])
}

// String returns a short label for logs.
func (c *Cache) String() string {
	return fmt.Sprintf("redis@%s", c.client.Options().Addr)
}
