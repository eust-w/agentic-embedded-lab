package computeruse

import (
	"context"
	"errors"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

type Decision string

const (
	DecisionAsk   Decision = "ask"
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

type Native interface {
	AccessibilityTrusted(prompt bool) bool
	ScreenRecordingTrusted(prompt bool) bool
	Click(x, y float64) error
	Type(text string) error
}

type Controller struct {
	state  *store.Store
	native Native
}

func New(state *store.Store, native Native) *Controller {
	return &Controller{state: state, native: native}
}

func (c *Controller) SetApplicationPermission(ctx context.Context, bundleID string, decision Decision, scope string) error {
	if bundleID == "" || (decision != DecisionAllow && decision != DecisionDeny) {
		return errors.New("bundle id and explicit decision are required")
	}
	_, err := c.state.DB().ExecContext(ctx, `INSERT INTO permissions(kind, resource, decision, scope, updated_at)
		VALUES ('application', ?, ?, ?, ?) ON CONFLICT(kind, resource) DO UPDATE SET
		decision=excluded.decision, scope=excluded.scope, updated_at=excluded.updated_at`, bundleID,
		decision, scope, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (c *Controller) ApplicationDecision(ctx context.Context, bundleID string) Decision {
	var decision Decision
	if err := c.state.DB().QueryRowContext(ctx, `SELECT decision FROM permissions
		WHERE kind='application' AND resource=?`, bundleID).Scan(&decision); err != nil {
		return DecisionAsk
	}
	return decision
}

func (c *Controller) Status(prompt bool) map[string]bool {
	return map[string]bool{
		"accessibility":    c.native.AccessibilityTrusted(prompt),
		"screen_recording": c.native.ScreenRecordingTrusted(prompt),
	}
}

func (c *Controller) Click(ctx context.Context, bundleID string, x, y float64) error {
	if c.ApplicationDecision(ctx, bundleID) != DecisionAllow {
		return errors.New("application control is not allowed")
	}
	if !c.native.AccessibilityTrusted(false) {
		return errors.New("macOS Accessibility permission is not granted")
	}
	return c.native.Click(x, y)
}

func (c *Controller) Type(ctx context.Context, bundleID, text string) error {
	if c.ApplicationDecision(ctx, bundleID) != DecisionAllow {
		return errors.New("application control is not allowed")
	}
	if !c.native.AccessibilityTrusted(false) {
		return errors.New("macOS Accessibility permission is not granted")
	}
	return c.native.Type(text)
}
