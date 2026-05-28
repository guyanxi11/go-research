package researcher

import (
	"reflect"
	"testing"

	"github.com/yourname/go-research/internal/tool"
)

func TestDedupAppend_PreservesInsertionOrder(t *testing.T) {
	seen := map[string]struct{}{}
	var ordered []tool.SearchItem

	round1 := []tool.SearchItem{
		{Title: "a", URL: "https://a.example"},
		{Title: "b", URL: "https://b.example"},
		{Title: "c", URL: "https://c.example"},
	}
	round2 := []tool.SearchItem{
		// b is duplicate, must be skipped silently.
		{Title: "b-again", URL: "https://b.example"},
		{Title: "d", URL: "https://d.example"},
		// empty URL must be skipped.
		{Title: "no-url"},
	}

	ordered = dedupAppend(seen, ordered, round1)
	ordered = dedupAppend(seen, ordered, round2)

	gotURLs := make([]string, 0, len(ordered))
	for _, it := range ordered {
		gotURLs = append(gotURLs, it.URL)
	}
	want := []string{
		"https://a.example",
		"https://b.example",
		"https://c.example",
		"https://d.example",
	}
	if !reflect.DeepEqual(gotURLs, want) {
		t.Errorf("order = %v, want %v", gotURLs, want)
	}
	// First-seen wins: the title for b must come from round1, not round2.
	if ordered[1].Title != "b" {
		t.Errorf("dedup kept later title %q, want first-seen 'b'", ordered[1].Title)
	}
}

func TestDedupAppend_StableAcrossRepeatedCalls(t *testing.T) {
	// Same inputs in same order must produce the same outputs every time
	// (regression for the previous non-deterministic map-based dedup).
	items := []tool.SearchItem{
		{Title: "1", URL: "https://1"},
		{Title: "2", URL: "https://2"},
		{Title: "3", URL: "https://3"},
		{Title: "4", URL: "https://4"},
		{Title: "5", URL: "https://5"},
	}

	var firstURLs []string
	for trial := 0; trial < 20; trial++ {
		seen := map[string]struct{}{}
		got := dedupAppend(seen, nil, items)
		got = dedupAppend(seen, got, items) // second pass: all dup
		urls := make([]string, 0, len(got))
		for _, it := range got {
			urls = append(urls, it.URL)
		}
		if trial == 0 {
			firstURLs = urls
			continue
		}
		if !reflect.DeepEqual(urls, firstURLs) {
			t.Fatalf("trial %d order changed: got %v, first %v", trial, urls, firstURLs)
		}
	}
}

func TestParseFollowUps_Clean(t *testing.T) {
	got := parseFollowUps(`{"follow_up_queries":["q1","  q2  ",""]}`)
	want := []string{"q1", "q2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseFollowUps_NoisyOutput(t *testing.T) {
	raw := "Sure!\n```json\n{\"follow_up_queries\": [\"hertz benchmark 2024\"]}\n```\n"
	got := parseFollowUps(raw)
	if len(got) != 1 || got[0] != "hertz benchmark 2024" {
		t.Errorf("got %v, want [hertz benchmark 2024]", got)
	}
}

func TestParseFollowUps_NoJSON(t *testing.T) {
	if got := parseFollowUps("no json here at all"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestParseFollowUps_InvalidJSON(t *testing.T) {
	if got := parseFollowUps(`{"follow_up_queries": broken`); got != nil {
		t.Errorf("got %v, want nil for invalid JSON", got)
	}
}

func TestParseFollowUps_Empty(t *testing.T) {
	if got := parseFollowUps(`{"follow_up_queries": []}`); got != nil {
		t.Errorf("got %v, want nil for empty array", got)
	}
}
