package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/google/uuid"
	rrule "github.com/teambition/rrule-go"
)

type Handler func(context.Context, protocol.AutomationSpec, string) error

type Scheduler struct {
	store    *store.Store
	handler  Handler
	workerID string
	interval time.Duration
	mu       sync.Mutex
	running  map[string]context.CancelFunc
}

func New(state *store.Store, handler Handler) *Scheduler {
	return &Scheduler{store: state, handler: handler, workerID: uuid.NewString(), interval: time.Second, running: make(map[string]context.CancelFunc)}
}

func (s *Scheduler) Save(ctx context.Context, spec protocol.AutomationSpec) error {
	if spec.ID == "" || spec.Name == "" || spec.ProjectID == "" || (spec.RRULE == "" && spec.EventSource == "") {
		return errors.New("automation id, name, project, and trigger are required")
	}
	next, err := nextRun(spec.RRULE, time.Now().UTC())
	if err != nil {
		return err
	}
	payload, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	_, err = s.store.DB().ExecContext(ctx, `INSERT INTO automations(id, spec_json, next_run_at, updated_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET spec_json=excluded.spec_json,
		next_run_at=excluded.next_run_at, updated_at=excluded.updated_at`, spec.ID, payload,
		nullableTime(next), formatTime(time.Now().UTC()))
	return err
}

func (s *Scheduler) List(ctx context.Context) ([]protocol.AutomationSpec, error) {
	rows, err := s.store.DB().QueryContext(ctx, `SELECT spec_json FROM automations ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var specs []protocol.AutomationSpec
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var spec protocol.AutomationSpec
		if err := json.Unmarshal(payload, &spec); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, rows.Err()
}

func (s *Scheduler) RunNow(ctx context.Context, automationID string) (string, error) {
	return s.enqueue(ctx, automationID, time.Now().UTC())
}

func (s *Scheduler) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.cancelAll()
			return ctx.Err()
		case now := <-ticker.C:
			if err := s.enqueueDue(ctx, now.UTC()); err != nil {
				return err
			}
		}
	}
}

func (s *Scheduler) enqueueDue(ctx context.Context, now time.Time) error {
	rows, err := s.store.DB().QueryContext(ctx, `SELECT id FROM automations
		WHERE next_run_at IS NOT NULL AND next_run_at <= ?`, formatTime(now))
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if _, err := s.enqueue(ctx, id, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) enqueue(ctx context.Context, automationID string, now time.Time) (string, error) {
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT spec_json FROM automations WHERE id = ?`, automationID).Scan(&payload); err != nil {
		return "", err
	}
	var spec protocol.AutomationSpec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return "", err
	}
	if !spec.Enabled {
		return "", errors.New("automation is disabled")
	}
	jobID := uuid.NewString()
	stamp := formatTime(now)
	if _, err := tx.ExecContext(ctx, `INSERT INTO automation_jobs
		(id, automation_id, status, created_at, updated_at) VALUES (?, ?, 'queued', ?, ?)`,
		jobID, automationID, stamp, stamp); err != nil {
		return "", err
	}
	next, err := nextRun(spec.RRULE, now)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE automations SET last_run_at = ?, next_run_at = ?, updated_at = ? WHERE id = ?`,
		stamp, nullableTime(next), stamp, automationID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	s.execute(spec, jobID)
	return jobID, nil
}

func (s *Scheduler) execute(spec protocol.AutomationSpec, jobID string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[jobID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, jobID)
			s.mu.Unlock()
		}()
		now := time.Now().UTC()
		lease := now.Add(2 * time.Minute)
		_, _ = s.store.DB().Exec(`UPDATE automation_jobs SET status='running', lease_owner=?,
			lease_expires_at=?, attempt=attempt+1, updated_at=? WHERE id=?`, s.workerID,
			formatTime(lease), formatTime(now), jobID)
		err := s.handler(ctx, spec, jobID)
		status := "completed"
		if err != nil {
			status = "failed"
		}
		_, _ = s.store.DB().Exec(`UPDATE automation_jobs SET status=?, lease_expires_at=NULL,
			updated_at=? WHERE id=?`, status, formatTime(time.Now().UTC()), jobID)
	}()
}

func (s *Scheduler) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.running {
		cancel()
		delete(s.running, id)
	}
}

func nextRun(rule string, after time.Time) (*time.Time, error) {
	if rule == "" {
		return nil, nil
	}
	parsed, err := rrule.StrToRRule(rule)
	if err != nil {
		return nil, fmt.Errorf("parse RRULE: %w", err)
	}
	next := parsed.After(after, false)
	if next.IsZero() {
		return nil, nil
	}
	next = next.UTC()
	return &next, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
