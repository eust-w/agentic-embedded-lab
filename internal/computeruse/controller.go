package computeruse

import (
	"context"
	"errors"
	"sync"
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
	FrontmostBundleID() (string, error)
	FocusedElementSecure() bool
	ElementTree(limit int) ([]byte, error)
	Screenshot() ([]byte, error)
	Click(x, y float64) error
	Type(text string) error
}

type Controller struct {
	state  *store.Store
	native Native
	mu     sync.Mutex
	once   map[string]bool
}

func New(state *store.Store, native Native) *Controller {
	return &Controller{state: state, native: native, once: make(map[string]bool)}
}

func (c *Controller) SetApplicationPermission(ctx context.Context, bundleID string, decision Decision, scope string) error {
	if bundleID == "" || (decision != DecisionAllow && decision != DecisionDeny) {
		return errors.New("bundle id and explicit decision are required")
	}
	if protectedApplication(bundleID) && decision == DecisionAllow {
		return errors.New("macOS security and login applications cannot be controlled")
	}
	if scope == "once" {
		c.mu.Lock()
		if decision == DecisionAllow {
			c.once[bundleID] = true
		} else {
			delete(c.once, bundleID)
		}
		c.mu.Unlock()
		return nil
	}
	_, err := c.state.DB().ExecContext(ctx, `INSERT INTO permissions(kind, resource, decision, scope, updated_at)
		VALUES ('application', ?, ?, ?, ?) ON CONFLICT(kind, resource) DO UPDATE SET
		decision=excluded.decision, scope=excluded.scope, updated_at=excluded.updated_at`, bundleID,
		decision, scope, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (c *Controller) ApplicationDecision(ctx context.Context, bundleID string) Decision {
	if protectedApplication(bundleID) {
		return DecisionDeny
	}
	c.mu.Lock()
	once := c.once[bundleID]
	c.mu.Unlock()
	if once {
		return DecisionAllow
	}
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
	if err := c.authorize(ctx, bundleID); err != nil {
		return err
	}
	return c.native.Click(x, y)
}

func (c *Controller) Type(ctx context.Context, bundleID, text string) error {
	if err := c.authorize(ctx, bundleID); err != nil {
		return err
	}
	if c.native.FocusedElementSecure() {
		return errors.New("typing into password or secure fields is prohibited")
	}
	return c.native.Type(text)
}

func (c *Controller) ElementTree(ctx context.Context, bundleID string, limit int) ([]byte, error) {
	if err := c.authorize(ctx, bundleID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return c.native.ElementTree(limit)
}

func (c *Controller) Screenshot(ctx context.Context, bundleID string) ([]byte, error) {
	if err := c.authorize(ctx, bundleID); err != nil {
		return nil, err
	}
	if !c.native.ScreenRecordingTrusted(false) {
		return nil, errors.New("macOS Screen Recording permission is not granted")
	}
	return c.native.Screenshot()
}

func (c *Controller) authorize(ctx context.Context, bundleID string) error {
	c.mu.Lock()
	once := c.once[bundleID]
	c.mu.Unlock()
	if !once && c.ApplicationDecision(ctx, bundleID) != DecisionAllow {
		return errors.New("application control is not allowed")
	}
	if !c.native.AccessibilityTrusted(false) {
		return errors.New("macOS Accessibility permission is not granted")
	}
	frontmost, err := c.native.FrontmostBundleID()
	if err != nil {
		return err
	}
	if frontmost != bundleID {
		return errors.New("authorized application is not frontmost")
	}
	if once {
		c.mu.Lock()
		delete(c.once, bundleID)
		c.mu.Unlock()
	}
	return nil
}

func protectedApplication(bundleID string) bool {
	switch bundleID {
	case "com.apple.systempreferences", "com.apple.SystemSettings", "com.apple.loginwindow", "com.apple.SecurityAgent":
		return true
	default:
		return false
	}
}
