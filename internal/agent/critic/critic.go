// Package critic scores Researcher findings and returns actionable feedback
// when quality is below threshold (Phase 4).
package critic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/yourname/go-research/internal/agent/jsonutil"
	"github.com/yourname/go-research/internal/llm"
)

// Review is the structured output of a Critic pass.
type Review struct {
	Score    int    `json:"score"`    // 1-10
	Pass     bool   `json:"pass"`     // true when score >= threshold
	Feedback string `json:"feedback"` // improvement hints when not passing
}

type Critic struct {
	llm      *llm.Client
	minScore int
}

func New(client *llm.Client, minScore int) *Critic {
	if minScore <= 0 {
		minScore = 6
	}
	return &Critic{llm: client, minScore: minScore}
}

const systemPrompt = `You are a strict research quality reviewer.

Score the researcher's findings on:
- coverage of the sub-question
- use of provided sources (citations)
- specificity (not vague filler)

Output a single JSON object only:
{
  "score": <integer 1-10>,
  "feedback": "<one short paragraph: what is missing or weak; empty string if excellent>"
}

Scoring guide: 8-10 solid, 6-7 acceptable but thin, 1-5 major gaps or ignores sources.`

// Review evaluates findings against the sub-question. Pass is derived from score
// and the configured minimum threshold.
func (c *Critic) Review(ctx context.Context, question, markdown string, citationCount int) (*Review, error) {
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("critic: empty findings")
	}
	prompt := fmt.Sprintf(`Sub-question:
%s

Researcher findings (Markdown):
%s

Number of citations attached: %d

Return JSON only.`, question, markdown, citationCount)

	out, err := c.llm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(prompt),
	})
	if err != nil {
		return nil, fmt.Errorf("critic llm: %w", err)
	}
	raw := strings.TrimSpace(out.Content)
	jsonStr := jsonutil.ExtractObject(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("critic: no JSON in response: %q", truncate(raw, 160))
	}
	var partial struct {
		Score    int    `json:"score"`
		Feedback string `json:"feedback"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &partial); err != nil {
		return nil, fmt.Errorf("critic decode: %w", err)
	}
	if partial.Score < 1 {
		partial.Score = 1
	}
	if partial.Score > 10 {
		partial.Score = 10
	}
	pass := partial.Score >= c.minScore
	return &Review{
		Score:    partial.Score,
		Pass:     pass,
		Feedback: strings.TrimSpace(partial.Feedback),
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
