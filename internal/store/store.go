package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schemaVersion = 2

type Store struct {
	db   *sql.DB
	root string
}

func Open(ctx context.Context, root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve store root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(abs, "cas", "sha256"), 0o700); err != nil {
		return nil, fmt.Errorf("create store root: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(abs, "aether.sqlite3"))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, root: abs}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS threads (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			title TEXT NOT NULL,
			model TEXT NOT NULL,
			permission TEXT NOT NULL,
			status TEXT NOT NULL,
			parent_id TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_threads_project_updated
			ON threads(project_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS turns (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
			status TEXT NOT NULL,
			input TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			error TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS items (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
			turn_id TEXT NOT NULL REFERENCES turns(id) ON DELETE CASCADE,
			sequence INTEGER NOT NULL,
			type TEXT NOT NULL,
			payload_json BLOB NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(thread_id, sequence)
		)`,
		`CREATE TABLE IF NOT EXISTS approvals (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			turn_id TEXT NOT NULL,
			tool TEXT NOT NULL,
			risk TEXT NOT NULL,
			scope TEXT NOT NULL,
			resource TEXT NOT NULL,
			decision TEXT,
			created_at TEXT NOT NULL,
			decided_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS tool_calls (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			turn_id TEXT NOT NULL,
			name TEXT NOT NULL,
			idempotency_key TEXT NOT NULL UNIQUE,
			arguments_json BLOB NOT NULL,
			result_json BLOB,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS automations (
			id TEXT PRIMARY KEY,
			spec_json BLOB NOT NULL,
			next_run_at TEXT,
			last_run_at TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS automation_jobs (
			id TEXT PRIMARY KEY,
			automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
			status TEXT NOT NULL,
			lease_owner TEXT,
			lease_expires_at TEXT,
			attempt INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(automation_id, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			scope TEXT NOT NULL,
			project_id TEXT,
			content TEXT NOT NULL,
			source_thread_id TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_scope_project
			ON memories(scope, project_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value_json BLOB NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS permissions (
			kind TEXT NOT NULL,
			resource TEXT NOT NULL,
			decision TEXT NOT NULL,
			scope TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(kind, resource)
		)`,
		`CREATE TABLE IF NOT EXISTS ael_runs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			status TEXT NOT NULL,
			request_json BLOB NOT NULL,
			result_json BLOB,
			error TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
	}
	for index, statement := range statements {
		var args []any
		if index == len(statements)-1 {
			args = []any{schemaVersion, time.Now().UTC().Format(time.RFC3339Nano)}
		}
		if _, err := s.db.ExecContext(ctx, statement, args...); err != nil {
			return fmt.Errorf("apply migration statement %d: %w", index, err)
		}
	}
	return nil
}

func (s *Store) CreateThread(ctx context.Context, projectID, title, model string, permission protocol.PermissionProfile) (protocol.Thread, error) {
	return s.createThread(ctx, projectID, title, model, permission, "")
}

func (s *Store) CreateChildThread(ctx context.Context, parentID, projectID, title, model string, permission protocol.PermissionProfile) (protocol.Thread, error) {
	if parentID == "" {
		return protocol.Thread{}, errors.New("parent thread id is required")
	}
	return s.createThread(ctx, projectID, title, model, permission, parentID)
}

func (s *Store) createThread(ctx context.Context, projectID, title, model string, permission protocol.PermissionProfile, parentID string) (protocol.Thread, error) {
	now := time.Now().UTC()
	thread := protocol.Thread{
		APIVersion: protocol.APIVersion,
		ID:         uuid.NewString(),
		ProjectID:  projectID,
		Title:      title,
		Model:      model,
		Permission: permission,
		Status:     protocol.ThreadReady,
		ParentID:   parentID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO threads
		(id, project_id, title, model, permission, status, parent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?)`, thread.ID, thread.ProjectID, thread.Title,
		thread.Model, thread.Permission, thread.Status, thread.ParentID, formatTime(now), formatTime(now))
	if err != nil {
		return protocol.Thread{}, fmt.Errorf("insert thread: %w", err)
	}
	return thread, nil
}

func (s *Store) ListThreads(ctx context.Context, projectID string) ([]protocol.Thread, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, title, model, permission,
		status, COALESCE(parent_id, ''), created_at, updated_at FROM threads
		WHERE project_id = ? ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	defer rows.Close()
	var result []protocol.Thread
	for rows.Next() {
		var thread protocol.Thread
		var created, updated string
		thread.APIVersion = protocol.APIVersion
		if err := rows.Scan(&thread.ID, &thread.ProjectID, &thread.Title, &thread.Model,
			&thread.Permission, &thread.Status, &thread.ParentID, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		thread.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		thread.UpdatedAt, err = parseTime(updated)
		if err != nil {
			return nil, err
		}
		result = append(result, thread)
	}
	return result, rows.Err()
}

func (s *Store) StartTurn(ctx context.Context, threadID, input string) (protocol.Turn, error) {
	now := time.Now().UTC()
	turn := protocol.Turn{
		APIVersion: protocol.APIVersion,
		ID:         uuid.NewString(),
		ThreadID:   threadID,
		Status:     protocol.ThreadRunning,
		Input:      input,
		StartedAt:  now,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return protocol.Turn{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO turns
		(id, thread_id, status, input, started_at) VALUES (?, ?, ?, ?, ?)`, turn.ID,
		turn.ThreadID, turn.Status, turn.Input, formatTime(now)); err != nil {
		return protocol.Turn{}, fmt.Errorf("insert turn: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE threads SET status = ?, updated_at = ? WHERE id = ?`,
		protocol.ThreadRunning, formatTime(now), threadID); err != nil {
		return protocol.Turn{}, fmt.Errorf("update thread: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return protocol.Turn{}, err
	}
	return turn, nil
}

func (s *Store) FinishTurn(ctx context.Context, threadID, turnID string, status protocol.ThreadStatus, message string) error {
	if status != protocol.ThreadCompleted && status != protocol.ThreadFailed && status != protocol.ThreadCancelled && status != protocol.ThreadWaiting {
		return errors.New("turn may only finish in a terminal or waiting state")
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE turns SET status=?, finished_at=?, error=NULLIF(?, '') WHERE id=? AND thread_id=? AND status=?`,
		status, formatTime(now), message, turnID, threadID, protocol.ThreadRunning)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return errors.New("running turn was not found")
	}
	threadStatus := status
	if status == protocol.ThreadCompleted {
		threadStatus = protocol.ThreadReady
	}
	if _, err := tx.ExecContext(ctx, `UPDATE threads SET status=?, updated_at=? WHERE id=?`, threadStatus, formatTime(now), threadID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AppendItem(ctx context.Context, item protocol.Item) (protocol.Item, error) {
	if item.ThreadID == "" || item.TurnID == "" || item.Type == "" {
		return protocol.Item{}, errors.New("thread_id, turn_id, and type are required")
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	item.APIVersion = protocol.APIVersion
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(item.Payload)
	if err != nil {
		return protocol.Item{}, fmt.Errorf("marshal item payload: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return protocol.Item{}, err
	}
	defer tx.Rollback()
	if item.Sequence == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1
			FROM items WHERE thread_id = ?`, item.ThreadID).Scan(&item.Sequence); err != nil {
			return protocol.Item{}, fmt.Errorf("allocate sequence: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO items
		(id, thread_id, turn_id, sequence, type, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, item.ID, item.ThreadID, item.TurnID, item.Sequence,
		item.Type, payload, formatTime(item.CreatedAt))
	if err != nil {
		return protocol.Item{}, fmt.Errorf("insert item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return protocol.Item{}, err
	}
	return item, nil
}

func (s *Store) Items(ctx context.Context, threadID string, after int64, limit int) ([]protocol.Item, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, turn_id, sequence, type, payload_json,
		created_at FROM items WHERE thread_id = ? AND sequence > ? ORDER BY sequence LIMIT ?`,
		threadID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []protocol.Item
	for rows.Next() {
		var item protocol.Item
		var payload []byte
		var created string
		item.APIVersion = protocol.APIVersion
		item.ThreadID = threadID
		if err := rows.Scan(&item.ID, &item.TurnID, &item.Sequence, &item.Type, &payload, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return nil, fmt.Errorf("decode item payload: %w", err)
		}
		item.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) BeginToolCall(ctx context.Context, call protocol.ToolCall) (bool, protocol.ToolResult, error) {
	arguments, err := json.Marshal(call.Arguments)
	if err != nil {
		return false, protocol.ToolResult{}, err
	}
	now := formatTime(time.Now().UTC())
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO tool_calls
		(id, thread_id, turn_id, name, idempotency_key, arguments_json, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'running', ?, ?)`, call.ID, call.ThreadID, call.TurnID,
		call.Name, call.IdempotencyKey, arguments, now, now)
	if err != nil {
		return false, protocol.ToolResult{}, err
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		return true, protocol.ToolResult{}, nil
	}
	var payload []byte
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status, COALESCE(result_json, '{}') FROM tool_calls
		WHERE idempotency_key = ?`, call.IdempotencyKey).Scan(&status, &payload); err != nil {
		return false, protocol.ToolResult{}, err
	}
	var cached protocol.ToolResult
	if status == "completed" || status == "failed" {
		if err := json.Unmarshal(payload, &cached); err != nil {
			return false, protocol.ToolResult{}, err
		}
		return false, cached, nil
	}
	return false, protocol.ToolResult{}, errors.New("tool call with this idempotency key is already running")
}

func (s *Store) FinishToolCall(ctx context.Context, idempotencyKey string, result protocol.ToolResult) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	status := "completed"
	if !result.Success {
		status = "failed"
	}
	updated, err := s.db.ExecContext(ctx, `UPDATE tool_calls SET status=?, result_json=?, updated_at=?
		WHERE idempotency_key=?`, status, payload, formatTime(time.Now().UTC()), idempotencyKey)
	if err != nil {
		return err
	}
	rows, _ := updated.RowsAffected()
	if rows != 1 {
		return errors.New("tool call was not found")
	}
	return nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
	}
	return parsed, nil
}
