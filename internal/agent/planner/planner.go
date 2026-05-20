// Package planner is the first stage of the research pipeline: it asks the
// LLM to decompose a user question into a small DAG of sub-questions.
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/yourname/go-research/internal/llm"
)

// Subtask is one node of the plan. depends_on references other subtask IDs;
// most plans should have empty depends_on so researchers run in parallel.
type Subtask struct {
	ID        string   `json:"id"`
	Question  string   `json:"question"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// Plan is the structured output of a Planner.
type Plan struct {
	Question string    `json:"question"`
	Subtasks []Subtask `json:"subtasks"`
}

// Planner is stateless; it owns only a reference to the shared LLM client.
type Planner struct {
	llm *llm.Client
}

func New(client *llm.Client) *Planner { return &Planner{llm: client} }

const systemPrompt = `You are the planning agent of a research pipeline.

Your job: decompose the user's question into 2 to 5 independent sub-questions that can each be
answered with one web search + brief synthesis, and that together cover the original question.

Output rules (STRICT):
- Output a single JSON object and nothing else. No prose, no markdown fences.
- Each sub-question must be answerable on its own; prefer parallel over sequential.
- Use depends_on ONLY when later questions truly need an earlier answer. Most plans should leave it empty.
- IDs are short slugs like "t1", "t2", "t3".

JSON shape:
{
  "question": "<echo of the user's question>",
  "subtasks": [
    {"id": "t1", "question": "...", "depends_on": []},
    {"id": "t2", "question": "...", "depends_on": []}
  ]
}`

// Plan asks the LLM to produce a plan for the given question. On JSON parse
// failure it retries once with a stronger reminder; if that also fails, it
// returns the parse error verbatim so the caller can surface it.
func (p *Planner) Plan(ctx context.Context, question string) (*Plan, error) {
	msgs := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("User question: " + question),
	}
	plan, firstErr := p.askOnce(ctx, msgs)
	if firstErr == nil {
		return plan, nil
	}
	// Retry once with the bad output appended and an explicit reminder.
	msgs = append(msgs,
		schema.AssistantMessage("(previous attempt was not valid JSON)", nil),
		schema.UserMessage("Re-output the plan as a single JSON object. No prose. No markdown fences."),
	)
	plan, secondErr := p.askOnce(ctx, msgs)
	if secondErr != nil {
		return nil, fmt.Errorf("planner: %w (retry: %v)", firstErr, secondErr)
	}
	return plan, nil
}

func (p *Planner) askOnce(ctx context.Context, msgs []*schema.Message) (*Plan, error) {
	out, err := p.llm.Generate(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	raw := strings.TrimSpace(out.Content)
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON object found in: %q", truncate(raw, 200))
	}
	var plan Plan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("decode plan: %w (body=%q)", err, truncate(jsonStr, 200))
	}
	if err := validate(&plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// extractJSON finds the outermost {...} block. Tolerates surrounding prose,
// markdown ```json fences, or chain-of-thought leaking past the system prompt.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```json ... ``` fences if present.
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func validate(p *Plan) error {
	if len(p.Subtasks) == 0 {
		return fmt.Errorf("plan has no subtasks")
	}
	if len(p.Subtasks) > 8 {
		return fmt.Errorf("plan has too many subtasks (%d)", len(p.Subtasks))
	}
	seen := make(map[string]bool, len(p.Subtasks))
	for i, s := range p.Subtasks {
		if s.ID == "" {
			return fmt.Errorf("subtask %d has empty id", i)
		}
		if s.Question == "" {
			return fmt.Errorf("subtask %q has empty question", s.ID)
		}
		if seen[s.ID] {
			return fmt.Errorf("duplicate subtask id %q", s.ID)
		}
		seen[s.ID] = true
	}
	for _, s := range p.Subtasks {
		for _, d := range s.DependsOn {
			if !seen[d] {
				return fmt.Errorf("subtask %q depends on unknown %q", s.ID, d)
			}
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
