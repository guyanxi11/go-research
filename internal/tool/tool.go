// Package tool defines the contract every Researcher-facing capability must
// satisfy (web search, page fetch, calculator, code execution, ...). Tools are
// intentionally string-in/string-out so they can be wired into a ReAct loop
// later without ABI churn.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Tool is the small surface a Researcher (or future ReAct loop) talks to.
type Tool interface {
	// Name is a short, stable identifier (e.g. "web_search").
	Name() string
	// Description is human-readable and may be shown to the LLM if the tool
	// is exposed via function-calling later.
	Description() string
	// Call performs the work. `args` is JSON for forward-compat; today most
	// callers pass {"query": "..."}. Return value is the tool's textual
	// output (search results joined, page text, ...).
	Call(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry is a tiny thread-safe lookup table.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

func (r *Registry) Register(t Tool) error {
	if t == nil {
		return errors.New("tool: nil")
	}
	name := t.Name()
	if name == "" {
		return errors.New("tool: empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.tools[name]; dup {
		return fmt.Errorf("tool: %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	return out
}

// SearchArgs is the canonical input shape every search-style tool accepts.
type SearchArgs struct {
	Query    string `json:"query"`
	MaxItems int    `json:"max_items,omitempty"`
}

// SearchResult is the canonical output. Researchers turn this into Markdown.
type SearchResult struct {
	Items []SearchItem `json:"items"`
}

type SearchItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}
