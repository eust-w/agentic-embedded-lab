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
  branch: string
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

export interface AgentHandle {
  id: string
  parent_id: string
  thread: Thread
  turn_id: string
  spec: AgentSpec
  status: 'active' | 'done' | 'failed' | 'interrupted'
  started_at: string
  updated_at: string
  worktree?: { path: string; branch: string; head: string }
}

export interface AgentResult {
  handle: AgentHandle
  items: Item[]
  summary: string
}

export interface HandoffResult {
  patch_sha256: string
  paths: string[]
  applied: boolean
  cleaned_up: boolean
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

export interface ReleaseResult {
  profile: 'foundation' | 'desktop' | 'agent' | 'simulation' | 'simulation-extensions' | 'software' | 'development-package' | 'production'
  passed: boolean
  failures: string[]
  checked: string[]
}

export interface AutomationJob {
  id: string
  automation_id: string
  status: 'queued' | 'running' | 'recovering' | 'waiting_for_approval' | 'completed' | 'failed' | 'cancelled'
  attempt: number
  created_at: string
  updated_at: string
}

export interface BrowserStatus {
  running: boolean
  executable: string
  url?: string
  title?: string
}

export interface BrowserConsoleEntry {
  level: string
  text: string
  timestamp: string
}

export interface BrowserNetworkEntry {
  method: string
  url: string
  status: number
  mime_type: string
  timestamp: string
}

export interface TerminalInfo {
  id: string
  workspace: string
  shell: string
  running: boolean
  exit_code: number
  created_at: string
}

export interface TerminalSnapshot extends TerminalInfo {
  offset: number
  next_offset: number
  data_base64: string
  truncated: boolean
}

export interface GitChange {
  path: string
  index: string
  worktree: string
}

export interface GitFileContent {
  path: string
  original: string
  modified: string
  language: string
}

export interface GitPullRequest {
  url: string
  number?: string
}

export interface AutomationSpec {
  api_version: 'aether.desktop/v1'
  id: string
  name: string
  prompt: string
  rrule?: string
  event_source?: string
  project_id: string
  use_worktree: boolean
  permission: PermissionProfile
  enabled: boolean
  stop_policy?: Record<string, unknown>
}

export interface InstalledPlugin {
  manifest: {
    api_version: string
    id: string
    name: string
    version: string
    description: string
    permissions: string[]
  }
  path: string
  active: boolean
  revoked: boolean
  installed_at: string
}

export interface AetherMemory {
  id: string
  scope: 'global' | 'project'
  project_id?: string
  content: string
  source_thread_id?: string
  created_at: string
  updated_at: string
}

export interface AttachmentRef {
  sha256: string
  name: string
  mime_type: string
  bytes: number
}
