package protocol

import "time"

const APIVersion = "aether.desktop/v1"

type ThreadStatus string

const (
	ThreadReady     ThreadStatus = "ready"
	ThreadRunning   ThreadStatus = "running"
	ThreadWaiting   ThreadStatus = "waiting_for_approval"
	ThreadFailed    ThreadStatus = "failed"
	ThreadCancelled ThreadStatus = "cancelled"
	ThreadCompleted ThreadStatus = "completed"
)

type PermissionProfile string

const (
	PermissionReadOnly   PermissionProfile = "read_only"
	PermissionWorkspace  PermissionProfile = "workspace_write"
	PermissionFullAccess PermissionProfile = "full_access"
)

type Thread struct {
	APIVersion string            `json:"api_version"`
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Title      string            `json:"title"`
	Model      string            `json:"model"`
	Permission PermissionProfile `json:"permission"`
	Status     ThreadStatus      `json:"status"`
	ParentID   string            `json:"parent_id,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type Turn struct {
	APIVersion string       `json:"api_version"`
	ID         string       `json:"id"`
	ThreadID   string       `json:"thread_id"`
	Status     ThreadStatus `json:"status"`
	Input      string       `json:"input"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	Error      string       `json:"error,omitempty"`
}

type ItemType string

const (
	ItemUserMessage  ItemType = "user_message"
	ItemAgentMessage ItemType = "agent_message"
	ItemReasoning    ItemType = "reasoning"
	ItemToolCall     ItemType = "tool_call"
	ItemToolResult   ItemType = "tool_result"
	ItemApproval     ItemType = "approval"
	ItemFileChange   ItemType = "file_change"
	ItemPlanUpdate   ItemType = "plan_update"
	ItemEvidence     ItemType = "evidence"
)

type Item struct {
	APIVersion string         `json:"api_version"`
	ID         string         `json:"id"`
	ThreadID   string         `json:"thread_id"`
	TurnID     string         `json:"turn_id"`
	Sequence   int64          `json:"sequence"`
	Type       ItemType       `json:"type"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ToolCall struct {
	ID             string         `json:"id"`
	ThreadID       string         `json:"thread_id"`
	TurnID         string         `json:"turn_id"`
	Name           string         `json:"name"`
	Arguments      map[string]any `json:"arguments"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type ToolResult struct {
	CallID      string         `json:"call_id"`
	Success     bool           `json:"success"`
	Output      map[string]any `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
	ArtifactIDs []string       `json:"artifact_ids,omitempty"`
	DurationMS  int64          `json:"duration_ms"`
}

type ApprovalScope string

const (
	ApprovalOnce    ApprovalScope = "once"
	ApprovalThread  ApprovalScope = "thread"
	ApprovalProject ApprovalScope = "project"
	ApprovalPersist ApprovalScope = "persistent"
)

type ApprovalRisk string

const (
	RiskLow      ApprovalRisk = "low"
	RiskMedium   ApprovalRisk = "medium"
	RiskHigh     ApprovalRisk = "high"
	RiskCritical ApprovalRisk = "critical"
)

type ApprovalRequest struct {
	APIVersion string         `json:"api_version"`
	ID         string         `json:"id"`
	ThreadID   string         `json:"thread_id"`
	TurnID     string         `json:"turn_id"`
	Tool       string         `json:"tool"`
	Reason     string         `json:"reason"`
	Risk       ApprovalRisk   `json:"risk"`
	Scope      ApprovalScope  `json:"scope"`
	Resource   string         `json:"resource"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AgentSpec struct {
	Name            string            `json:"name"`
	Role            string            `json:"role"`
	Model           string            `json:"model"`
	ReasoningEffort string            `json:"reasoning_effort"`
	Permission      PermissionProfile `json:"permission"`
	Tools           []string          `json:"tools"`
	MaxConcurrency  int               `json:"max_concurrency"`
}

type WorktreeRef struct {
	ID          string    `json:"id"`
	Repository  string    `json:"repository"`
	Path        string    `json:"path"`
	BaseBranch  string    `json:"base_branch"`
	Head        string    `json:"head"`
	PatchSHA256 string    `json:"patch_sha256,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type AutomationSpec struct {
	APIVersion  string            `json:"api_version"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Prompt      string            `json:"prompt"`
	RRULE       string            `json:"rrule,omitempty"`
	EventSource string            `json:"event_source,omitempty"`
	ProjectID   string            `json:"project_id"`
	UseWorktree bool              `json:"use_worktree"`
	Permission  PermissionProfile `json:"permission"`
	Enabled     bool              `json:"enabled"`
	StopPolicy  map[string]any    `json:"stop_policy,omitempty"`
}
