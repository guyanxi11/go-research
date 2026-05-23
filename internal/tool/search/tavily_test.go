package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourname/go-research/internal/tool"
)

func TestTavily_Call(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tvly-test" {
			t.Fatalf("Authorization = %q", got)
		}
		var body tavilyReq
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Query != "goroutines" || body.MaxResults != 2 {
			t.Fatalf("unexpected body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"title": "Example", "url": "https://example.com", "content": "snippet text"}
			]
		}`))
	}))
	defer srv.Close()

	tav := &Tavily{
		apiKey:      "tvly-test",
		searchDepth: "basic",
		http:        srv.Client(),
		endpoint:    srv.URL,
	}
	args, _ := json.Marshal(tool.SearchArgs{Query: "goroutines", MaxItems: 2})
	out, err := tav.Call(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	var sr tool.SearchResult
	if err := json.Unmarshal([]byte(out), &sr); err != nil {
		t.Fatal(err)
	}
	if len(sr.Items) != 1 || sr.Items[0].URL != "https://example.com" {
		t.Fatalf("unexpected result: %+v", sr)
	}
}

func TestTavily_EmptyKey(t *testing.T) {
	_, err := NewTavily("", "").Call(context.Background(), json.RawMessage(`{"query":"go"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
