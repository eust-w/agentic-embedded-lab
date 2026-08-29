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
