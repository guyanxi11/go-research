package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/yourname/go-research/internal/store"
)

func (s *Store) CreateSession(ctx context.Context, id, question string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO research_sessions (id, question, status)
		VALUES ($1, $2, 'running')
	`, id, question)
	return err
}

func (s *Store) SavePlan(ctx context.Context, sessionID string, plan json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE research_sessions SET plan = $2 WHERE id = $1
	`, sessionID, plan)
	return err
}

func (s *Store) UpsertTask(ctx context.Context, sessionID string, rec store.TaskRecord) error {
	cites := rec.Citations
	if len(cites) == 0 {
		cites = json.RawMessage("[]")
	}
	// sort_order is intentionally INSERT-only: it is seeded once at EventPlan
	// time and must not be overwritten by later node_started / node_finished
	// upserts (which carry the Go zero value 0 and would otherwise corrupt
	// the persisted plan order).
	_, err := s.pool.Exec(ctx, `
		INSERT INTO research_tasks (
			session_id, task_id, question, status, findings, citations,
			elapsed_ms, error_message, sort_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (session_id, task_id) DO UPDATE SET
			question = EXCLUDED.question,
			status = EXCLUDED.status,
			findings = EXCLUDED.findings,
			citations = EXCLUDED.citations,
			elapsed_ms = EXCLUDED.elapsed_ms,
			error_message = EXCLUDED.error_message
	`, sessionID, rec.TaskID, rec.Question, rec.Status,
		rec.Findings, cites, rec.ElapsedMs, nullIfEmpty(rec.ErrorMessage), rec.SortOrder)
	return err
}

func (s *Store) CompleteSession(
	ctx context.Context,
	sessionID string,
	status store.SessionStatus,
	report string,
	elapsedMs int64,
	charCount int,
	errMsg string,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE research_sessions SET
			status = $2,
			report = $3,
			elapsed_ms = $4,
			char_count = $5,
			error_message = $6,
			completed_at = NOW()
		WHERE id = $1
	`, sessionID, status, report, elapsedMs, charCount, nullIfEmpty(errMsg))
	return err
}

func (s *Store) ListSessions(ctx context.Context, limit, offset int) ([]store.SessionSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, question, status, created_at, elapsed_ms, char_count, completed_at
		FROM research_sessions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.SessionSummary
	for rows.Next() {
		var sum store.SessionSummary
		if err := rows.Scan(
			&sum.ID, &sum.Question, &sum.Status, &sum.CreatedAt,
			&sum.ElapsedMs, &sum.CharCount, &sum.CompletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, rows.Err()
}

func (s *Store) GetSession(ctx context.Context, id string) (*store.SessionDetail, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, question, status, created_at, elapsed_ms, char_count,
		       completed_at, plan, report, error_message
		FROM research_sessions
		WHERE id = $1
	`, id)

	var det store.SessionDetail
	var plan []byte
	var errMsg *string
	if err := row.Scan(
		&det.ID, &det.Question, &det.Status, &det.CreatedAt,
		&det.ElapsedMs, &det.CharCount, &det.CompletedAt,
		&plan, &det.Report, &errMsg,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if len(plan) > 0 {
		det.Plan = json.RawMessage(plan)
	}
	if errMsg != nil {
		det.ErrorMessage = *errMsg
	}

	taskRows, err := s.pool.Query(ctx, `
		SELECT task_id, question, status, findings, citations, elapsed_ms, error_message, sort_order
		FROM research_tasks
		WHERE session_id = $1
		ORDER BY sort_order, task_id
	`, id)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var t store.TaskRecord
		var cites []byte
		var taskErr *string
		if err := taskRows.Scan(
			&t.TaskID, &t.Question, &t.Status, &t.Findings, &cites,
			&t.ElapsedMs, &taskErr, &t.SortOrder,
		); err != nil {
			return nil, err
		}
		if len(cites) > 0 {
			t.Citations = json.RawMessage(cites)
		}
		if taskErr != nil {
			t.ErrorMessage = *taskErr
		}
		det.Tasks = append(det.Tasks, t)
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}
	return &det, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
