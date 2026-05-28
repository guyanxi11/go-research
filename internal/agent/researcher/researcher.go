// Package researcher answers a single sub-question using bounded multi-round
// search (ReAct-style) and optional Critic review with resynthesis (Phase 4).
package researcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yourname/go-research/internal/agent/critic"
	"github.com/yourname/go-research/internal/llm"
	"github.com/yourname/go-research/internal/metrics"
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
	Index   int    `json:"index"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type Researcher struct {
	llm    *llm.Client
	search tool.Tool
	opts   Options
	critic *critic.Critic
}

func New(client *llm.Client, search tool.Tool, opts Options) *Researcher {
	opts = opts.normalized()
	var cr *critic.Critic
	if opts.CriticEnabled {
		cr = critic.New(client, opts.CriticMinScore)
	}
	return &Researcher{llm: client, search: search, opts: opts, critic: cr}
}

// Research runs search → synthesis → optional Critic loop.
// ProgressHook is optional (used by orchestrator for SSE).
func (r *Researcher) Research(
	ctx context.Context,
	taskID, question string,
	upstream []*Findings,
	hook ProgressHook,
) (out *Findings, retErr error) {
	start := time.Now()
	defer func() {
		metrics.AgentStepDurationSeconds.
			WithLabelValues("researcher", metrics.Outcome(retErr)).
			Observe(time.Since(start).Seconds())
	}()
	items, err := r.collectSearchResults(ctx, taskID, question, r.opts, hook)
	if err != nil {
		return nil, err
	}

	maxAttempts := 1 + r.opts.MaxCriticRetries
	var feedback string
	var last *Findings

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		f, err := r.synthesize(ctx, question, items, upstream, feedback)
		if err != nil {
			return nil, err
		}
		f.TaskID = taskID
		f.Question = question
		last = f

		if r.critic == nil {
			return f, nil
		}

		review, err := r.critic.Review(ctx, question, f.Markdown, len(f.Citations))
		if err != nil {
			return nil, fmt.Errorf("critic: %w", err)
		}
		if hook != nil {
			hook(ProgressEvent{CriticReview: &CriticReviewPayload{
				TaskID: taskID, Attempt: attempt,
				Score: review.Score, Pass: review.Pass, Feedback: review.Feedback,
			}})
		}
		if review.Pass || attempt == maxAttempts {
			return f, nil
		}
		feedback = review.Feedback
	}
	return last, nil
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
