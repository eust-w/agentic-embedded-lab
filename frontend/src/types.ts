export type WorkspaceView = 'chat' | 'diff' | 'browser' | 'simulation'

export interface ThreadSummary {
  id: string
  title: string
  subtitle: string
  updated: string
}

export interface AgentStatus {
  id: string
  name: string
  role: string
  status: 'working' | 'running' | 'idle'
  progress: number
  detail: string
  worktree: string
  tone: 'blue' | 'violet' | 'green'
}

export interface Approval {
  id: string
  title: string
  description: string
  resources: string[]
  additions: number
  deletions: number
  status: 'pending' | 'approved' | 'denied'
}

export type PermissionProfile = 'read_only' | 'workspace_write' | 'full_access'

export interface ToolDefinition {
  type: string
  name: string
  description: string
  parameters: Record<string, unknown>
}

export interface ProjectInfo {
  id: string
  root: string
  permission: PermissionProfile
  tools: ToolDefinition[]
}

export interface Thread {
  api_version: string
  id: string
  project_id: string
  title: string
  model: string
  permission: PermissionProfile
  status: 'ready' | 'running' | 'waiting_for_approval' | 'failed' | 'cancelled' | 'completed'
  parent_id?: string
  created_at: string
  updated_at: string
}

export interface Turn {
  api_version: string
  id: string
  thread_id: string
  status: Thread['status']
  input: string
  started_at: string
  finished_at?: string
  error?: string
}

export interface Item {
  api_version: string
  id: string
  thread_id: string
  turn_id: string
  sequence: number
  type: 'user_message' | 'agent_message' | 'reasoning' | 'tool_call' | 'tool_result' | 'approval' | 'file_change' | 'plan_update' | 'evidence'
  payload: Record<string, unknown>
  created_at: string
}

export interface AgentSpec {
  name: string
  role: string
  model: string
  reasoning_effort: string
  permission: PermissionProfile
  tools: string[]
  max_concurrency: number
}

export interface AELRunRequest {
  project_id: string
  experiment_path: string
  system_path: string
  source_revision: string
}

export interface AELEvent {
  sequence: number
  virtual_time_us: number
  source: string
  type: string
  payload: Record<string, unknown>
  fidelity_ref: string
}

export interface AELEvidenceBundle {
  run_id: string
  trace_sha256: string
  events: AELEvent[]
  assertions: Array<{ id: string; passed: boolean; observed: number; expected: number; message: string }>
  artifacts: Record<string, string>
  fidelity: { firmware: string; register: string; protocol: string; timing: string; physical: string; hardware_validated: boolean; limitations: string[] }
  failure?: { code: string; message: string; retryable: boolean }
}

export interface AELRunRecord {
  id: string
  request: AELRunRequest
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled'
  evidence_path?: string
  bundle?: AELEvidenceBundle
  error?: string
  created_at: string
  updated_at: string
}
