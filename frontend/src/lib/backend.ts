import type { AELRunRecord, AELRunRequest, AgentSpec, BrowserConsoleEntry, BrowserNetworkEntry, BrowserStatus, Item, PermissionProfile, ProjectInfo, ReleaseResult, Thread, Turn } from '../types'

export interface BackendAPI {
  Health(): Promise<{ status: string; time: string }>
  InstallBackgroundService(): Promise<void>
  UpdateStatus(): Promise<{ available: boolean; started: boolean }>
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
  BrowserStatus(): Promise<BrowserStatus>
  StartBrowser(): Promise<void>
  StopBrowser(): Promise<void>
  SetSitePermission(url: string, allow: boolean): Promise<void>
  RevokeSitePermission(url: string): Promise<void>
  NavigateBrowser(url: string): Promise<void>
  BrowserDOM(): Promise<string>
  BrowserScreenshot(): Promise<string>
  BrowserConsole(after: number): Promise<BrowserConsoleEntry[]>
  BrowserNetwork(after: number): Promise<BrowserNetworkEntry[]>
  LatestChromeSnapshot(): Promise<{ available: boolean; snapshot?: { id: string; tab_id: number; url: string; title: string; dom: string; captured_at: string } }>
  BrowserClick(selector: string, confirmed: boolean): Promise<void>
  BrowserType(selector: string, text: string): Promise<void>
}

declare global {
  interface Window {
    go?: { app?: { Backend?: BackendAPI } }
  }
}

export function backend(): BackendAPI | null {
  return window.go?.app?.Backend ?? null
}
