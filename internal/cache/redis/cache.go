// Package redis provides optional Redis-backed caching for tool calls.
package redis

import (
	"context"
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

// GetSearch returns cached JSON for an opaque key, or ("", false, nil).
//
// The key is treated as opaque: the caller is responsible for composing it
// (e.g. across provider/depth/max_items dimensions). This namespace only
// prepends a fixed "search:" prefix.
func (c *Cache) GetSearch(ctx context.Context, key string) (string, bool, error) {
	val, err := c.client.Get(ctx, redisKey(key)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// SetSearch stores search tool output JSON under an opaque key.
func (c *Cache) SetSearch(ctx context.Context, key, jsonResult string) error {
	return c.client.Set(ctx, redisKey(key), jsonResult, c.ttl).Err()
}

func redisKey(key string) string {
	return "search:" + key
}

// String returns a short label for logs.
func (c *Cache) String() string {
	return fmt.Sprintf("redis@%s", c.client.Options().Addr)
}
