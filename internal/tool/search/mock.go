// Package search provides search-style tools for the Researcher agent.
//
// Mock is the fallback used when no real search API key is configured. It
// produces deterministic, citation-shaped placeholders so the end-to-end
// pipeline can be developed and demoed without any external dependency.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/yourname/go-research/internal/tool"
)

type Mock struct{}

func NewMock() *Mock { return &Mock{} }

func (*Mock) Name() string { return "web_search" }
func (*Mock) Description() string {
	return "Search the web for the given query. Returns a list of {title, url, snippet}."
}

func (m *Mock) Call(_ context.Context, args json.RawMessage) (string, error) {
	var a tool.SearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("mock search: bad args: %w", err)
	}
	if a.Query == "" {
		return "", fmt.Errorf("mock search: empty query")
	}
	if a.MaxItems <= 0 {
		a.MaxItems = 3
	}

	out := tool.SearchResult{Items: make([]tool.SearchItem, 0, a.MaxItems)}
	encoded := url.QueryEscape(a.Query)
	for i := 0; i < a.MaxItems; i++ {
		out.Items = append(out.Items, tool.SearchItem{
			Title: fmt.Sprintf("[mock #%d] %s", i+1, truncate(a.Query, 60)),
			URL:   "https://example.com/mock?q=" + encoded + "&n=" + itoa(i+1),
			Snippet: fmt.Sprintf(
				"Mock excerpt #%d for the query %q. The Researcher should treat this as a placeholder citation "+
					"and synthesise findings from the LLM's own knowledge. Set TAVILY_API_KEY in .env to use real search.",
				i+1, a.Query),
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}
