package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yourname/go-research/internal/tool"
)

// Tavily is a thin client for https://tavily.com/'s search API. Free tier is
// ~1000 calls/month which is plenty for Phase 2.B demoing. If the key is
// empty, callers should fall back to Mock.
type Tavily struct {
	apiKey      string
	searchDepth string
	http        *http.Client
	endpoint    string // empty → https://api.tavily.com/search (tests may override)
}

// NewTavily builds a Tavily search tool. searchDepth is "basic" (default) or
// "advanced" (higher quality, uses more API credits per Tavily billing).
func NewTavily(apiKey string, searchDepth string) *Tavily {
	if searchDepth == "" {
		searchDepth = "basic"
	}
	return &Tavily{
		apiKey:      apiKey,
		searchDepth: searchDepth,
		http:        &http.Client{Timeout: 20 * time.Second},
	}
}

func (*Tavily) Name() string { return "web_search" }
func (*Tavily) Description() string {
	return "Search the web via Tavily. Returns a list of {title, url, snippet}."
}

// tavilyReq mirrors https://docs.tavily.com/documentation/api-reference/endpoint/search
// Auth is via Authorization: Bearer header (not api_key in body).
type tavilyReq struct {
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth,omitempty"`
	MaxResults  int    `json:"max_results,omitempty"`
}

type tavilyResp struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
	Error string `json:"error,omitempty"`
}

func (t *Tavily) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if t.apiKey == "" {
		return "", fmt.Errorf("tavily: api key not configured")
	}
	var a tool.SearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("tavily: bad args: %w", err)
	}
	if a.Query == "" {
		return "", fmt.Errorf("tavily: empty query")
	}
	if a.MaxItems <= 0 {
		a.MaxItems = 5
	}

	depth := t.searchDepth
	if depth == "" {
		depth = "basic"
	}
	body, _ := json.Marshal(tavilyReq{
		Query:       a.Query,
		SearchDepth: depth,
		MaxResults:  a.MaxItems,
	})
	url := t.endpoint
	if url == "" {
		url = "https://api.tavily.com/search"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("tavily: request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("tavily: http %d: %s", resp.StatusCode, snippet(raw, 200))
	}

	var tr tavilyResp
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", fmt.Errorf("tavily: decode: %w (body=%s)", err, snippet(raw, 200))
	}
	if tr.Error != "" {
		return "", fmt.Errorf("tavily: %s", tr.Error)
	}

	out := tool.SearchResult{Items: make([]tool.SearchItem, 0, len(tr.Results))}
	for _, r := range tr.Results {
		out.Items = append(out.Items, tool.SearchItem{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func snippet(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
