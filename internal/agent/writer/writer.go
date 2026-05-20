// Package writer is the last stage of the pipeline: it composes a coherent
// Markdown report from all Researcher findings and streams tokens back.
package writer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/yourname/go-research/internal/agent/researcher"
	"github.com/yourname/go-research/internal/llm"
)

type Writer struct {
	llm *llm.Client
}

func New(client *llm.Client) *Writer { return &Writer{llm: client} }

const systemPrompt = `You are the writer agent of a research pipeline. Compose a coherent Markdown
report that answers the user's original question, drawing on the researchers' findings.

Rules:
- Output well-structured Markdown with sections: "# <title>", "## Key takeaways" (3-5 bullets),
  "## Detailed findings" (group by sub-topic), "## References".
- Preserve and renumber citations across the whole report (e.g. [1], [2], ...).
- Do not invent facts beyond what the researchers found.
- Write in the same language as the user's original question.`

// TokenHandler receives each incremental token chunk. Return an error to abort
// the stream; the underlying StreamReader will be closed.
type TokenHandler func(delta string) error

// Stream produces the final report token-by-token. It returns when the stream
// ends or the handler returns an error. The full assembled text is returned
// for callers who also want to persist it (e.g. to a database).
func (w *Writer) Stream(
	ctx context.Context,
	question string,
	findings []*researcher.Findings,
	onToken TokenHandler,
) (string, error) {
	if len(findings) == 0 {
		return "", errors.New("writer: no findings to summarise")
	}

	prompt := buildPrompt(question, findings)
	stream, err := w.llm.Stream(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(prompt),
	})
	if err != nil {
		return "", fmt.Errorf("writer: stream init: %w", err)
	}
	defer stream.Close()

	var full strings.Builder
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return full.String(), fmt.Errorf("writer: stream recv: %w", err)
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}
		full.WriteString(chunk.Content)
		if onToken != nil {
			if err := onToken(chunk.Content); err != nil {
				return full.String(), err
			}
		}
	}
	return full.String(), nil
}

func buildPrompt(question string, findings []*researcher.Findings) string {
	var b strings.Builder
	b.WriteString("Original user question:\n")
	b.WriteString(question)
	b.WriteString("\n\n")

	// Build a consolidated citation table the LLM can renumber off of.
	type citeRow struct {
		title, url string
	}
	var cites []citeRow
	seen := map[string]int{}
	for _, f := range findings {
		for _, c := range f.Citations {
			if _, ok := seen[c.URL]; !ok {
				seen[c.URL] = len(cites) + 1
				cites = append(cites, citeRow{c.Title, c.URL})
			}
		}
	}

	b.WriteString("Researcher findings:\n\n")
	for _, f := range findings {
		fmt.Fprintf(&b, "### Sub-question: %s\n", f.Question)
		b.WriteString(f.Markdown)
		b.WriteString("\n\n")
	}

	b.WriteString("Consolidated references (renumber in your output):\n")
	for i, c := range cites {
		fmt.Fprintf(&b, "[%d] %s — %s\n", i+1, c.title, c.url)
	}
	b.WriteString("\nWrite the final Markdown report now.")
	return b.String()
}
