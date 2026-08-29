package memory

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/google/uuid"
)

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

type Memory struct {
	ID             string    `json:"id"`
	Scope          Scope     `json:"scope"`
	ProjectID      string    `json:"project_id,omitempty"`
	Content        string    `json:"content"`
	SourceThreadID string    `json:"source_thread_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Repository struct{ store *store.Store }

func New(state *store.Store) *Repository { return &Repository{store: state} }

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*[^\s,;]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
}

func Redact(content string) string {
	for _, pattern := range secretPatterns {
		content = pattern.ReplaceAllString(content, "[REDACTED]")
	}
	return content
}

func (r *Repository) Save(ctx context.Context, memory Memory) (Memory, error) {
	memory.Content = strings.TrimSpace(Redact(memory.Content))
	if memory.Content == "" {
		return Memory{}, errors.New("memory content is required")
	}
	if memory.Scope != ScopeGlobal && memory.Scope != ScopeProject {
		return Memory{}, errors.New("memory scope must be global or project")
	}
	if memory.Scope == ScopeProject && memory.ProjectID == "" {
		return Memory{}, errors.New("project memory requires project id")
	}
	now := time.Now().UTC()
	if memory.ID == "" {
		memory.ID = uuid.NewString()
		memory.CreatedAt = now
	}
	memory.UpdatedAt = now
	_, err := r.store.DB().ExecContext(ctx, `INSERT INTO memories
		(id, scope, project_id, content, source_thread_id, created_at, updated_at)
		VALUES (?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?)
		ON CONFLICT(id) DO UPDATE SET content=excluded.content, updated_at=excluded.updated_at`,
		memory.ID, memory.Scope, memory.ProjectID, memory.Content, memory.SourceThreadID,
		memory.CreatedAt.Format(time.RFC3339Nano), memory.UpdatedAt.Format(time.RFC3339Nano))
	return memory, err
}

func (r *Repository) Search(ctx context.Context, scope Scope, projectID, query string, limit int) ([]Memory, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.store.DB().QueryContext(ctx, `SELECT id, scope, COALESCE(project_id, ''),
		content, COALESCE(source_thread_id, ''), created_at, updated_at FROM memories
		WHERE scope = ? AND (? = '' OR project_id = ?) AND content LIKE ?
		ORDER BY updated_at DESC LIMIT ?`, scope, projectID, projectID, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Memory
	for rows.Next() {
		var memory Memory
		var created, updated string
		if err := rows.Scan(&memory.ID, &memory.Scope, &memory.ProjectID, &memory.Content,
			&memory.SourceThreadID, &created, &updated); err != nil {
			return nil, err
		}
		memory.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		memory.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, memory)
	}
	return result, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.store.DB().ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return errors.New("memory not found")
	}
	return nil
}
