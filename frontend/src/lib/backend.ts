import type { AELRunRecord, AELRunRequest, AgentSpec, Item, PermissionProfile, ProjectInfo, ReleaseResult, Thread, Turn } from '../types'

export interface BackendAPI {
  Health(): Promise<{ status: string; time: string }>
  InstallBackgroundService(): Promise<void>
  SelectProject(permission: PermissionProfile): Promise<ProjectInfo>
  ListThreads(projectID: string): Promise<Thread[]>
  CreateThread(projectID: string, title: string, permission: PermissionProfile): Promise<Thread>
  Items(threadID: string, after: number): Promise<Item[]>
  RunTurn(thread: Thread, input: string): Promise<Turn>
  CancelTurn(turnID: string): Promise<boolean>
  ResolveApproval(approvalID: string, allow: boolean): Promise<boolean>
  SpawnAgent(parent: Thread, prompt: string, spec: AgentSpec): Promise<unknown>
  StartExperiment(request: AELRunRequest): Promise<AELRunRecord>
  GetExperiment(id: string): Promise<AELRunRecord>
  CancelExperiment(id: string): Promise<boolean>
  CheckRelease(profile: ReleaseResult['profile']): Promise<ReleaseResult>
}

declare global {
  interface Window {
    go?: { app?: { Backend?: BackendAPI } }
  }
}

export function backend(): BackendAPI | null {
  return window.go?.app?.Backend ?? null
}
