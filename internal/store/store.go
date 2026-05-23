package store

import (
	"context"
	"encoding/json"
	"time"
)

// SessionStatus is persisted on research_sessions.status.
type SessionStatus string

const (
	StatusRunning   SessionStatus = "running"
	StatusCompleted SessionStatus = "completed"
	StatusFailed    SessionStatus = "failed"
)

// TaskStatus is persisted on research_tasks.status.
type TaskStatus string

const (
	TaskPending TaskStatus = "pending"
	TaskRunning TaskStatus = "running"
	TaskDone    TaskStatus = "done"
	TaskFailed  TaskStatus = "failed"
)

// SessionSummary is returned by list endpoints.
type SessionSummary struct {
	ID         string        `json:"id"`
	Question   string        `json:"question"`
	Status     SessionStatus `json:"status"`
	CreatedAt  time.Time     `json:"created_at"`
	ElapsedMs  *int64        `json:"elapsed_ms,omitempty"`
	CharCount  *int          `json:"char_count,omitempty"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
}

// TaskRecord is one Researcher subtask outcome.
type TaskRecord struct {
	TaskID       string          `json:"task_id"`
	Question     string          `json:"question"`
	Status       TaskStatus      `json:"status"`
	Findings     string          `json:"findings,omitempty"`
	Citations    json.RawMessage `json:"citations,omitempty"`
	ElapsedMs    *int64          `json:"elapsed_ms,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	SortOrder    int             `json:"sort_order"`
}

// SessionDetail is the full persisted research run.
type SessionDetail struct {
	SessionSummary
	Plan         json.RawMessage `json:"plan,omitempty"`
	Report       string          `json:"report,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Tasks        []TaskRecord    `json:"tasks"`
}

// ResearchStore persists research pipeline runs.
type ResearchStore interface {
	CreateSession(ctx context.Context, id, question string) error
	SavePlan(ctx context.Context, sessionID string, plan json.RawMessage) error
	UpsertTask(ctx context.Context, sessionID string, rec TaskRecord) error
	CompleteSession(ctx context.Context, sessionID string, status SessionStatus, report string, elapsedMs int64, charCount int, errMsg string) error
	ListSessions(ctx context.Context, limit, offset int) ([]SessionSummary, error)
	GetSession(ctx context.Context, id string) (*SessionDetail, error)
	Close()
}
