import type { AELRunRecord, AELRunRequest, AgentHandle, AgentSpec, AetherMemory, AttachmentRef, AutomationSpec, BrowserConsoleEntry, BrowserNetworkEntry, BrowserStatus, GitChange, GitFileContent, InstalledPlugin, Item, PermissionProfile, ProjectInfo, ReleaseResult, TerminalInfo, TerminalSnapshot, Thread, Turn } from '../types'

export interface BackendAPI {
  Health(): Promise<{ status: string; time: string }>
  InstallBackgroundService(): Promise<void>
  UpdateStatus(): Promise<{ available: boolean; started: boolean }>
  SelectProject(permission: PermissionProfile): Promise<ProjectInfo>
  ListThreads(projectID: string): Promise<Thread[]>
  CreateThread(projectID: string, title: string, permission: PermissionProfile): Promise<Thread>
  Items(threadID: string, after: number): Promise<Item[]>
  RunTurn(thread: Thread, input: string): Promise<Turn>
  RunTurnWithAttachments(thread: Thread, input: string, attachments: AttachmentRef[]): Promise<Turn>
  PickImageAttachments(): Promise<AttachmentRef[]>
  CancelTurn(turnID: string): Promise<boolean>
  ResolveApproval(approvalID: string, allow: boolean): Promise<boolean>
  SpawnAgent(parent: Thread, prompt: string, spec: AgentSpec): Promise<AgentHandle>
  ListAgents(): Promise<AgentHandle[]>
  InterruptAgent(id: string): Promise<boolean>
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
  ComputerStatus(prompt: boolean): Promise<{ accessibility: boolean; screen_recording: boolean }>
  ComputerDecision(bundleID: string): Promise<{ bundle_id: string; decision: 'ask' | 'allow' | 'deny' }>
  SetComputerPermission(bundleID: string, allow: boolean, scope: string): Promise<void>
  StartTerminal(columns: number, rows: number): Promise<TerminalInfo>
  ListTerminals(): Promise<TerminalInfo[]>
  ReadTerminal(id: string, after: number): Promise<TerminalSnapshot>
  WriteTerminal(id: string, dataBase64: string): Promise<void>
  ResizeTerminal(id: string, columns: number, rows: number): Promise<void>
  StopTerminal(id: string): Promise<void>
  GitChanges(scope: string, base: string): Promise<GitChange[]>
  GitFileContent(path: string, scope: string, base: string): Promise<GitFileContent>
  GitStage(paths: string[]): Promise<void>
  GitUnstage(paths: string[]): Promise<void>
  GitRestore(paths: string[]): Promise<void>
  StartCodeReview(scope: string, base: string): Promise<Thread>
  SaveAutomation(spec: AutomationSpec): Promise<AutomationSpec>
  ListAutomations(): Promise<AutomationSpec[]>
  RunAutomation(id: string): Promise<string>
  CancelAutomation(jobID: string): Promise<boolean>
  ListPlugins(): Promise<InstalledPlugin[]>
  SelectAndInstallPlugin(approvePermissions: boolean): Promise<InstalledPlugin>
  RevokePlugin(id: string, reason: string): Promise<void>
  MemoryStatus(): Promise<{ global: boolean; project: boolean }>
  SetMemoryEnabled(scope: 'global' | 'project', enabled: boolean): Promise<void>
  ListMemories(scope: 'global' | 'project'): Promise<AetherMemory[]>
  SaveMemory(scope: 'global' | 'project', content: string, sourceThreadID: string): Promise<AetherMemory>
  DeleteMemory(id: string): Promise<void>
}

declare global {
  interface Window {
    go?: { app?: { Backend?: BackendAPI } }
  }
}

export function backend(): BackendAPI | null {
  return window.go?.app?.Backend ?? null
}
