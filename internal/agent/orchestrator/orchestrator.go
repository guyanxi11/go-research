// Package orchestrator runs the end-to-end research pipeline:
// Planner -> DAG of Researchers -> Writer.
//
// It emits a single, unified Event stream so the HTTP layer (and any future
// transport: gRPC, CLI, ...) can stay completely agnostic of the internal
// stage machinery.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yourname/go-research/internal/agent/dag"
	"github.com/yourname/go-research/internal/agent/planner"
	"github.com/yourname/go-research/internal/agent/researcher"
	"github.com/yourname/go-research/internal/agent/writer"
	"github.com/yourname/go-research/internal/llm"
	"github.com/yourname/go-research/internal/tool"
)

// EventType is a string alias so the JSON wire format is human-readable.
type EventType string

const (
	EventPlan         EventType = "plan"
	EventNodeStarted  EventType = "node_started"
	EventNodeFinished EventType = "node_finished"
	EventNodeFailed   EventType = "node_failed"
	EventWriterToken  EventType = "writer_token"
	EventDone         EventType = "done"
	EventError        EventType = "error"
)

// Event is the orchestrator's outgoing wire format. Payload is shape-typed by
// Type. SSE / WebSocket layers can JSON-encode it directly.
type Event struct {
	Type    EventType `json:"type"`
	Payload any       `json:"payload,omitempty"`
}

// Specific payload structs used in Event.Payload.

type PlanPayload struct {
	Question string            `json:"question"`
	Subtasks []planner.Subtask `json:"subtasks"`
}

type NodeStartedPayload struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Attempt  int    `json:"attempt"`
}

type NodeFinishedPayload struct {
	ID        string                `json:"id"`
	Question  string                `json:"question"`
	Findings  string                `json:"findings"`
	Citations []researcher.Citation `json:"citations"`
	ElapsedMs int64                 `json:"elapsed_ms"`
}

type NodeFailedPayload struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type WriterTokenPayload struct {
	Delta string `json:"delta"`
}

type DonePayload struct {
	ElapsedMs int64 `json:"elapsed_ms"`
	Chars     int   `json:"chars"`
}

// Deps groups the runtime dependencies an Engine needs.
type Deps struct {
	LLM       *llm.Client
	Tools     *tool.Registry
	Scheduler *dag.Scheduler
}

type Engine struct {
	deps Deps
}

func New(deps Deps) *Engine { return &Engine{deps: deps} }

// Run executes the full pipeline. Events flow to `events` strictly in order:
//
//	plan -> (node_started, node_finished | node_failed)* -> writer_token* -> done | error
//
// The function blocks until the pipeline finishes or ctx is cancelled. The
// caller owns the events channel and is responsible for closing it when the
// returned function exits.
func (e *Engine) Run(ctx context.Context, question string, events chan<- Event) error {
	if question == "" {
		return errors.New("orchestrator: empty question")
	}
	if e.deps.LLM == nil || e.deps.Tools == nil || e.deps.Scheduler == nil {
		return errors.New("orchestrator: missing dependency")
	}

	overallStart := time.Now()

	// ---- Stage 1: Planner -----------------------------------------------
	p := planner.New(e.deps.LLM)
	plan, err := p.Plan(ctx, question)
	if err != nil {
		emit(events, Event{Type: EventError, Payload: map[string]string{"stage": "planner", "error": err.Error()}})
		return err
	}
	emit(events, Event{Type: EventPlan, Payload: PlanPayload{Question: plan.Question, Subtasks: plan.Subtasks}})

	// ---- Stage 2: DAG of Researchers -----------------------------------
	searchTool, ok := e.deps.Tools.Get("web_search")
	if !ok {
		err := errors.New("orchestrator: web_search tool not registered")
		emit(events, Event{Type: EventError, Payload: map[string]string{"stage": "tools", "error": err.Error()}})
		return err
	}
	r := researcher.New(e.deps.LLM, searchTool)

	g := dag.NewGraph()
	questionByID := make(map[string]string, len(plan.Subtasks))
	for _, st := range plan.Subtasks {
		st := st // capture by value
		questionByID[st.ID] = st.Question
		err := g.Add(&dag.Node{
			ID:        st.ID,
			Name:      st.Question,
			DependsOn: st.DependsOn,
			MaxRetry:  1,
			Timeout:   90 * time.Second,
			Run: func(ctx context.Context, deps map[string]any) (any, error) {
				upstream := make([]*researcher.Findings, 0, len(deps))
				for _, v := range deps {
					if f, ok := v.(*researcher.Findings); ok {
						upstream = append(upstream, f)
					}
				}
				return r.Research(ctx, st.ID, st.Question, upstream)
			},
		})
		if err != nil {
			emit(events, Event{Type: EventError, Payload: map[string]string{"stage": "dag", "error": err.Error()}})
			return err
		}
	}

	// Bridge dag.Event -> orchestrator.Event in a sidecar goroutine.
	dagEvents := make(chan dag.Event, 256)
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		for de := range dagEvents {
			switch de.Type {
			case dag.EventNodeStarted:
				emit(events, Event{Type: EventNodeStarted, Payload: NodeStartedPayload{
					ID: de.NodeID, Question: questionByID[de.NodeID], Attempt: de.Attempt,
				}})
			case dag.EventNodeSucceeded:
				f, _ := de.Result.(*researcher.Findings)
				if f == nil {
					continue
				}
				emit(events, Event{Type: EventNodeFinished, Payload: NodeFinishedPayload{
					ID: de.NodeID, Question: f.Question,
					Findings: f.Markdown, Citations: f.Citations,
					ElapsedMs: de.Elapsed.Milliseconds(),
				}})
			case dag.EventNodeFailed:
				msg := ""
				if de.Error != nil {
					msg = de.Error.Error()
				}
				emit(events, Event{Type: EventNodeFailed, Payload: NodeFailedPayload{
					ID: de.NodeID, Error: msg,
				}})
			}
		}
	}()

	res, runErr := e.deps.Scheduler.Run(ctx, g, dagEvents)
	close(dagEvents)
	<-bridgeDone
	if runErr != nil {
		emit(events, Event{Type: EventError, Payload: map[string]string{"stage": "scheduler", "error": runErr.Error()}})
		return runErr
	}

	// Collect findings in original plan order so the report flows naturally.
	findings := make([]*researcher.Findings, 0, len(plan.Subtasks))
	for _, st := range plan.Subtasks {
		f, ok := res.Outputs[st.ID].(*researcher.Findings)
		if !ok {
			continue
		}
		findings = append(findings, f)
	}
	if len(findings) == 0 {
		err := errors.New("orchestrator: no researcher produced findings")
		emit(events, Event{Type: EventError, Payload: map[string]string{"stage": "scheduler", "error": err.Error()}})
		return err
	}

	// ---- Stage 3: Writer (streaming) -----------------------------------
	w := writer.New(e.deps.LLM)
	chars := 0
	full, werr := w.Stream(ctx, question, findings, func(delta string) error {
		chars += len(delta)
		emit(events, Event{Type: EventWriterToken, Payload: WriterTokenPayload{Delta: delta}})
		return ctx.Err()
	})
	if werr != nil {
		emit(events, Event{Type: EventError, Payload: map[string]string{"stage": "writer", "error": werr.Error()}})
		return werr
	}
	_ = full // returned for callers that want to persist the final report

	emit(events, Event{Type: EventDone, Payload: DonePayload{
		ElapsedMs: time.Since(overallStart).Milliseconds(),
		Chars:     chars,
	}})
	return nil
}

// emit pushes an event with blocking semantics so writer tokens are never
// dropped. The events channel is expected to be generously buffered (the SSE
// handler uses 1024) and drained by a fast consumer (HTTP writer).
func emit(ch chan<- Event, e Event) {
	if ch == nil {
		return
	}
	ch <- e
}

// Errors that callers may wish to distinguish.
var (
	ErrEmptyQuestion = errors.New("orchestrator: empty question")
)

// helper kept for diagnostics
func sprintfEvent(e Event) string { return fmt.Sprintf("%s %+v", e.Type, e.Payload) }
