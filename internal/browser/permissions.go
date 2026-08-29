package browser

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

type Decision string

const (
	DecisionAsk   Decision = "ask"
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

type PermissionStore struct{ state *store.Store }

func NewPermissionStore(state *store.Store) *PermissionStore { return &PermissionStore{state: state} }

func (p *PermissionStore) Set(ctx context.Context, kind, resource string, decision Decision, scope string) error {
	if kind == "" || resource == "" || (decision != DecisionAllow && decision != DecisionDeny) {
		return errors.New("valid permission kind, resource, and decision are required")
	}
	_, err := p.state.DB().ExecContext(ctx, `INSERT INTO permissions(kind, resource, decision, scope, updated_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(kind, resource) DO UPDATE SET
		decision=excluded.decision, scope=excluded.scope, updated_at=excluded.updated_at`, kind, resource,
		decision, scope, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (p *PermissionStore) Site(ctx context.Context, rawURL string) (Decision, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return DecisionDeny, errors.New("invalid site URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return DecisionAllow, nil
	}
	var decision Decision
	err = p.state.DB().QueryRowContext(ctx, `SELECT decision FROM permissions WHERE kind='site' AND resource=?`, host).Scan(&decision)
	if err != nil {
		return DecisionAsk, nil
	}
	return decision, nil
}

func (p *PermissionStore) Revoke(ctx context.Context, kind, resource string) error {
	_, err := p.state.DB().ExecContext(ctx, `DELETE FROM permissions WHERE kind=? AND resource=?`, kind, resource)
	return err
}
