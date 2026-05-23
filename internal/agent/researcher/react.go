package researcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/yourname/go-research/internal/agent/jsonutil"
	"github.com/yourname/go-research/internal/tool"
)

// collectSearchResults runs bounded ReAct-style search waves and merges snippets.
func (r *Researcher) collectSearchResults(
	ctx context.Context,
	taskID, question string,
	opts Options,
	hook ProgressHook,
) ([]tool.SearchItem, error) {
	opts = opts.normalized()
	byURL := make(map[string]tool.SearchItem)
	queries := []string{question}

	for round := 1; round <= opts.MaxSearchRounds; round++ {
		var nextQueries []string
		for _, q := range queries {
			items, err := r.searchOnce(ctx, q)
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				if it.URL != "" {
					byURL[it.URL] = it
				}
			}
			if hook != nil {
				hook(ProgressEvent{SearchRound: &SearchRoundPayload{
					TaskID: taskID, Round: round, Query: q, ResultCount: len(items),
				}})
			}
		}
		if round >= opts.MaxSearchRounds {
			break
		}
		followUps, err := r.planFollowUpQueries(ctx, question, mapValues(byURL))
		if err != nil {
			return nil, err
		}
		if len(followUps) == 0 {
			break
		}
		if len(followUps) > opts.MaxFollowUpQueries {
			followUps = followUps[:opts.MaxFollowUpQueries]
		}
		nextQueries = followUps
		queries = nextQueries
	}

	out := make([]tool.SearchItem, 0, len(byURL))
	for _, it := range byURL {
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no search results for %q", question)
	}
	return out, nil
}

func (r *Researcher) searchOnce(ctx context.Context, query string) ([]tool.SearchItem, error) {
	args, _ := json.Marshal(tool.SearchArgs{Query: query, MaxItems: 5})
	raw, err := r.search.Call(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	var results tool.SearchResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}
	return results.Items, nil
}

const followUpSystem = `You decide whether more web searches are needed.

Given the sub-question and snippets already collected, output JSON only:
{"follow_up_queries":["..."]}

Rules:
- Return at most 2 short search queries.
- Return an empty array if existing snippets are enough.
- No prose, no markdown fences.`

func (r *Researcher) planFollowUpQueries(ctx context.Context, question string, items []tool.SearchItem) ([]string, error) {
	var b strings.Builder
	b.WriteString("Sub-question:\n")
	b.WriteString(question)
	b.WriteString("\n\nCollected snippets:\n")
	for i, it := range items {
		if i >= 8 {
			b.WriteString("... (truncated)\n")
			break
		}
		fmt.Fprintf(&b, "- %s: %s\n", it.Title, oneLine(it.Snippet, 120))
	}
	out, err := r.llm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(followUpSystem),
		schema.UserMessage(b.String()),
	})
	if err != nil {
		return nil, err
	}
	jsonStr := jsonutil.ExtractObject(strings.TrimSpace(out.Content))
	if jsonStr == "" {
		return nil, nil
	}
	var parsed struct {
		FollowUpQueries []string `json:"follow_up_queries"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, nil
	}
	var clean []string
	for _, q := range parsed.FollowUpQueries {
		q = strings.TrimSpace(q)
		if q != "" {
			clean = append(clean, q)
		}
	}
	return clean, nil
}

func mapValues(m map[string]tool.SearchItem) []tool.SearchItem {
	out := make([]tool.SearchItem, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func (r *Researcher) synthesize(ctx context.Context, question string, items []tool.SearchItem, upstream []*Findings, extraContext string) (*Findings, error) {
	prompt := buildPrompt(question, items, upstream)
	if extraContext != "" {
		prompt += "\n\nReviewer feedback (address these gaps):\n" + extraContext + "\n"
	}
	out, err := r.llm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(prompt),
	})
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	cites := make([]Citation, len(items))
	for i, it := range items {
		cites[i] = Citation{Index: i + 1, Title: it.Title, URL: it.URL, Snippet: it.Snippet}
	}
	return &Findings{
		Question:  question,
		Markdown:  strings.TrimSpace(out.Content),
		Citations: cites,
	}, nil
}
