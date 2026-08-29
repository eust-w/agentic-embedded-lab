package approval

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

type Operation struct {
	ThreadID    string
	ProjectID   string
	Tool        string
	Action      string
	Resource    string
	Risk        protocol.ApprovalRisk
	Network     bool
	External    bool
	Destructive bool
}

type Rule struct {
	ID        string
	ProjectID string
	Tool      string
	Action    string
	Prefix    string
	Decision  Decision
	Scope     protocol.ApprovalScope
}

type Result struct {
	Decision Decision
	Reason   string
	RuleID   string
}

type Engine struct {
	mu    sync.RWMutex
	rules []Rule
}

func New() *Engine { return &Engine{} }

func (e *Engine) AddRule(rule Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

func (e *Engine) Evaluate(profile protocol.PermissionProfile, workspace string, operation Operation) Result {
	if operation.Destructive || operation.External || operation.Risk == protocol.RiskCritical {
		return Result{Decision: DecisionAsk, Reason: "sensitive operation requires explicit approval"}
	}
	if operation.Network {
		return Result{Decision: DecisionAsk, Reason: "network access is disabled by default"}
	}
	if pathLike(operation.Resource) && !withinWorkspace(workspace, operation.Resource) {
		if profile == protocol.PermissionFullAccess {
			return Result{Decision: DecisionAsk, Reason: "resource is outside the workspace"}
		}
		return Result{Decision: DecisionDeny, Reason: "resource escapes the workspace"}
	}
	if rule, ok := e.match(operation); ok {
		return Result{Decision: rule.Decision, Reason: "matched policy rule", RuleID: rule.ID}
	}
	switch profile {
	case protocol.PermissionReadOnly:
		if operation.Action == "read" || operation.Action == "inspect" {
			return Result{Decision: DecisionAllow, Reason: "read-only operation"}
		}
		return Result{Decision: DecisionDeny, Reason: "profile is read-only"}
	case protocol.PermissionWorkspace:
		if operation.Risk == protocol.RiskHigh {
			return Result{Decision: DecisionAsk, Reason: "high-risk workspace operation"}
		}
		return Result{Decision: DecisionAllow, Reason: "operation is scoped to workspace"}
	case protocol.PermissionFullAccess:
		if operation.Risk == protocol.RiskHigh {
			return Result{Decision: DecisionAsk, Reason: "high-risk operation requires confirmation"}
		}
		return Result{Decision: DecisionAllow, Reason: "full-access profile"}
	default:
		return Result{Decision: DecisionDeny, Reason: "unknown permission profile"}
	}
}

func (e *Engine) match(operation Operation) (Rule, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for index := len(e.rules) - 1; index >= 0; index-- {
		rule := e.rules[index]
		if rule.ProjectID != "" && rule.ProjectID != operation.ProjectID {
			continue
		}
		if rule.Tool != "" && rule.Tool != operation.Tool {
			continue
		}
		if rule.Action != "" && rule.Action != operation.Action {
			continue
		}
		if rule.Prefix != "" && !strings.HasPrefix(operation.Resource, rule.Prefix) {
			continue
		}
		return rule, true
	}
	return Rule{}, false
}

func withinWorkspace(workspace, target string) bool {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	resolved, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func pathLike(resource string) bool {
	return strings.HasPrefix(resource, "/") || strings.HasPrefix(resource, ".")
}
