package server

import (
	"context"
	"encoding/json"
	"sync"

	hlog "github.com/cloudwego/hertz/pkg/common/hlog"

	"github.com/yourname/go-research/internal/agent/orchestrator"
	"github.com/yourname/go-research/internal/store"
)

// researchRecorder persists orchestrator events to Postgres.
type researchRecorder struct {
	store     store.ResearchStore
	sessionID string
	mu        sync.Mutex

	planJSON   json.RawMessage
	report     string
	elapsedMs  int64
	charCount  int
	sortOrder  int
	pipelineOK bool
	failMsg    string
}

func newResearchRecorder(st store.ResearchStore, sessionID string) *researchRecorder {
	return &researchRecorder{store: st, sessionID: sessionID}
}

func (r *researchRecorder) OnEvent(ctx context.Context, ev orchestrator.Event) {
	if r == nil || r.store == nil {
		return
	}
	switch ev.Type {
	case orchestrator.EventPlan:
		p, ok := ev.Payload.(orchestrator.PlanPayload)
		if !ok {
			return
		}
		raw, err := json.Marshal(p)
		if err != nil {
			hlog.Warnf("research recorder: marshal plan: %v", err)
			return
		}
		r.mu.Lock()
		r.planJSON = raw
		r.mu.Unlock()
		if err := r.store.SavePlan(ctx, r.sessionID, raw); err != nil {
			hlog.Warnf("research recorder: save plan: %v", err)
		}
		for i, st := range p.Subtasks {
			rec := store.TaskRecord{
				TaskID:    st.ID,
				Question:  st.Question,
				Status:    store.TaskPending,
				SortOrder: i,
			}
			if err := r.store.UpsertTask(ctx, r.sessionID, rec); err != nil {
				hlog.Warnf("research recorder: seed task %s: %v", st.ID, err)
			}
		}

	case orchestrator.EventNodeStarted:
		p, ok := ev.Payload.(orchestrator.NodeStartedPayload)
		if !ok {
			return
		}
		rec := store.TaskRecord{
			TaskID:   p.ID,
			Question: p.Question,
			Status:   store.TaskRunning,
		}
		if err := r.store.UpsertTask(ctx, r.sessionID, rec); err != nil {
			hlog.Warnf("research recorder: task running %s: %v", p.ID, err)
		}

	case orchestrator.EventNodeFinished:
		p, ok := ev.Payload.(orchestrator.NodeFinishedPayload)
		if !ok {
			return
		}
		cites, _ := json.Marshal(p.Citations)
		elapsed := p.ElapsedMs
		rec := store.TaskRecord{
			TaskID:    p.ID,
			Question:  p.Question,
			Status:    store.TaskDone,
			Findings:  p.Findings,
			Citations: cites,
			ElapsedMs: &elapsed,
		}
		if err := r.store.UpsertTask(ctx, r.sessionID, rec); err != nil {
			hlog.Warnf("research recorder: task done %s: %v", p.ID, err)
		}

	case orchestrator.EventNodeFailed:
		p, ok := ev.Payload.(orchestrator.NodeFailedPayload)
		if !ok {
			return
		}
		rec := store.TaskRecord{
			TaskID:       p.ID,
			Status:       store.TaskFailed,
			ErrorMessage: p.Error,
		}
		if err := r.store.UpsertTask(ctx, r.sessionID, rec); err != nil {
			hlog.Warnf("research recorder: task failed %s: %v", p.ID, err)
		}

	case orchestrator.EventWriterToken:
		p, ok := ev.Payload.(orchestrator.WriterTokenPayload)
		if !ok {
			return
		}
		r.mu.Lock()
		r.report += p.Delta
		r.mu.Unlock()

	case orchestrator.EventDone:
		p, ok := ev.Payload.(orchestrator.DonePayload)
		if !ok {
			return
		}
		r.mu.Lock()
		r.pipelineOK = true
		r.elapsedMs = p.ElapsedMs
		r.charCount = p.Chars
		r.mu.Unlock()

	case orchestrator.EventError:
		if m, ok := ev.Payload.(map[string]string); ok {
			r.mu.Lock()
			r.failMsg = m["error"]
			if stage := m["stage"]; stage != "" {
				r.failMsg = stage + ": " + r.failMsg
			}
			r.mu.Unlock()
		}
	}
}

func (r *researchRecorder) Finalize(ctx context.Context, pipelineErr error) {
	if r == nil || r.store == nil {
		return
	}
	r.mu.Lock()
	report := r.report
	elapsed := r.elapsedMs
	chars := r.charCount
	ok := r.pipelineOK && pipelineErr == nil
	failMsg := r.failMsg
	if pipelineErr != nil && failMsg == "" {
		failMsg = pipelineErr.Error()
	}
	r.mu.Unlock()

	status := store.StatusCompleted
	if !ok {
		status = store.StatusFailed
	}
	if err := r.store.CompleteSession(ctx, r.sessionID, status, report, elapsed, chars, failMsg); err != nil {
		hlog.Warnf("research recorder: complete session: %v", err)
	}
}
