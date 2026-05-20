// Package researcher answers a single sub-question by calling one search tool
// once and then asking the LLM to synthesise findings + inline citations.
//
// Why no ReAct loop? It is intentional: Phase 2.B prioritises predictable,
// cheap, debuggable execution over open-ended tool use. The LLM is constrained
// to "synthesise from the snippets I gave you" which keeps token spend and
// latency bounded and avoids the model inventing tool calls. ReAct lives on
// the roadmap for Phase 2.5+ once the rest of the pipeline is solid.
package researcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/yourname/go-research/internal/llm"
	"github.com/yourname/go-research/internal/tool"
)

// Findings is what one Researcher produces for one sub-question.
type Findings struct {
	TaskID    string     `json:"task_id"`
	Question  string     `json:"question"`
	Markdown  string     `json:"markdown"`
	Citations []Citation `json:"citations"`
}

type Citation struct {
	Index   int    `json:"index"` // [1], [2], ... as referenced in Markdown
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type Researcher struct {
	llm    *llm.Client
	search tool.Tool
}

func New(client *llm.Client, search tool.Tool) *Researcher {
	return &Researcher{llm: client, search: search}
}

// Research is the entry point used by the orchestrator and the DAG node.
// `upstream` carries Findings from dependency subtasks (currently unused in
// the prompt but accepted so future Planner outputs can chain reasoning).
func (r *Researcher) Research(ctx context.Context, taskID, question string, upstream []*Findings) (*Findings, error) {
	args, _ := json.Marshal(tool.SearchArgs{Query: question, MaxItems: 5})
	rawResults, err := r.search.Call(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	var results tool.SearchResult
	if err := json.Unmarshal([]byte(rawResults), &results); err != nil {
		return nil, fmt.Errorf("decode search results: %w", err)
	}
	if len(results.Items) == 0 {
		return nil, fmt.Errorf("no search results for %q", question)
	}

	prompt := buildPrompt(question, results.Items, upstream)
	out, err := r.llm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(prompt),
	})
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}

	cites := make([]Citation, len(results.Items))
	for i, it := range results.Items {
		cites[i] = Citation{Index: i + 1, Title: it.Title, URL: it.URL, Snippet: it.Snippet}
	}
	return &Findings{
		TaskID:    taskID,
		Question:  question,
		Markdown:  strings.TrimSpace(out.Content),
		Citations: cites,
	}, nil
}

const systemPrompt = `You are a research agent. Answer the sub-question using ONLY the search results
provided. Write 2-4 short paragraphs of Markdown. Cite sources inline using [1], [2], ... that map
1-to-1 with the search result indices. If the snippets are insufficient, say so plainly. Do NOT
repeat the sub-question.`

func buildPrompt(question string, items []tool.SearchItem, upstream []*Findings) string {
	var b strings.Builder
	b.WriteString("Sub-question:\n")
	b.WriteString(question)
	b.WriteString("\n\nSearch results:\n")
	for i, it := range items {
		fmt.Fprintf(&b, "[%d] %s (%s)\n%s\n\n", i+1, it.Title, it.URL, it.Snippet)
	}
	if len(upstream) > 0 {
		b.WriteString("Context from upstream researchers (use sparingly):\n")
		for _, u := range upstream {
			fmt.Fprintf(&b, "- %s: %s\n", u.Question, oneLine(u.Markdown, 240))
		}
		b.WriteString("\n")
	}
	b.WriteString("Output ONLY the Markdown findings.")
	return b.String()
}

func oneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}
