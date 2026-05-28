package search

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/yourname/go-research/internal/tool"
)

// fakeCache records get/set keys so we can assert on key composition.
type fakeCache struct {
	store map[string]string
	gets  []string
	sets  []string
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: map[string]string{}}
}

func (c *fakeCache) GetSearch(_ context.Context, key string) (string, bool, error) {
	c.gets = append(c.gets, key)
	v, ok := c.store[key]
	return v, ok, nil
}

func (c *fakeCache) SetSearch(_ context.Context, key, v string) error {
	c.sets = append(c.sets, key)
	c.store[key] = v
	return nil
}

// fakeTool returns a canned JSON response and counts how often Call is invoked.
type fakeTool struct {
	name  string
	resp  string
	calls int
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return "fake" }
func (f *fakeTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	f.calls++
	return f.resp, nil
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCached_CacheHit(t *testing.T) {
	inner := &fakeTool{name: "web_search", resp: `{"items":[]}`}
	cache := newFakeCache()
	c := NewCached(inner, cache, "mock")

	args := mustMarshal(t, tool.SearchArgs{Query: "go concurrency", MaxItems: 5})

	if _, err := c.Call(context.Background(), args); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("first call: inner.calls = %d, want 1", inner.calls)
	}
	if _, err := c.Call(context.Background(), args); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("second call should hit cache: inner.calls = %d, want still 1", inner.calls)
	}
}

func TestCached_DifferentNamespacesDoNotShare(t *testing.T) {
	// Same query, same args, but different provider/depth must miss.
	cache := newFakeCache()
	innerA := &fakeTool{name: "web_search", resp: `{"items":["A"]}`}
	innerB := &fakeTool{name: "web_search", resp: `{"items":["B"]}`}

	cA := NewCached(innerA, cache, "tavily:basic")
	cB := NewCached(innerB, cache, "tavily:advanced")
	args := mustMarshal(t, tool.SearchArgs{Query: "same query", MaxItems: 5})

	if _, err := cA.Call(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if _, err := cB.Call(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if innerA.calls != 1 || innerB.calls != 1 {
		t.Errorf("namespaces should not share cache: A=%d B=%d", innerA.calls, innerB.calls)
	}
}

func TestCached_DifferentMaxItemsDoNotShare(t *testing.T) {
	cache := newFakeCache()
	inner := &fakeTool{name: "web_search", resp: `{"items":[]}`}
	c := NewCached(inner, cache, "mock")

	a := mustMarshal(t, tool.SearchArgs{Query: "q", MaxItems: 3})
	b := mustMarshal(t, tool.SearchArgs{Query: "q", MaxItems: 8})

	if _, err := c.Call(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 2 {
		t.Errorf("different max_items should not share cache: inner.calls = %d, want 2", inner.calls)
	}
}

func TestCached_EmptyQueryBypassesCache(t *testing.T) {
	cache := newFakeCache()
	inner := &fakeTool{name: "web_search", resp: `{"items":[]}`}
	c := NewCached(inner, cache, "mock")

	args := mustMarshal(t, tool.SearchArgs{MaxItems: 5})
	if _, err := c.Call(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if len(cache.gets) != 0 || len(cache.sets) != 0 {
		t.Errorf("empty query should bypass cache, gets=%v sets=%v", cache.gets, cache.sets)
	}
}

func TestCached_PropagatesInnerError(t *testing.T) {
	cache := newFakeCache()
	failing := &errTool{err: errors.New("boom")}
	c := NewCached(failing, cache, "mock")
	args := mustMarshal(t, tool.SearchArgs{Query: "q", MaxItems: 5})
	if _, err := c.Call(context.Background(), args); err == nil {
		t.Fatal("expected error to propagate")
	}
	if len(cache.sets) != 0 {
		t.Errorf("must not cache failures: %v", cache.sets)
	}
}

type errTool struct{ err error }

func (e *errTool) Name() string        { return "web_search" }
func (e *errTool) Description() string { return "errTool" }
func (e *errTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	return "", e.err
}
