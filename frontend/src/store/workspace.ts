import { create } from 'zustand'
import { backend } from '../lib/backend'
import type { AELRunRecord, AgentHandle, AgentStatus, Approval, AttachmentRef, AutomationJob, AutomationSpec, InstalledPlugin, Item, ProjectInfo, ReleaseResult, Thread, ThreadSummary, WorkspaceView } from '../types'

interface WorkspaceState {
  view: WorkspaceView
  running: boolean
  selectedThread: string
  model: string
  desiredPermission: 'read_only' | 'workspace_write' | 'full_access'
  inspectorTab: 'context' | 'agents' | 'evidence'
  approval: Approval
  threads: ThreadSummary[]
  agents: AgentStatus[]
  liveAgents: AgentHandle[]
  automations: AutomationSpec[]
  plugins: InstalledPlugin[]
  runningAutomationJob?: string
  automationJob?: AutomationJob
  agentResults: Record<string, string>
  handoffResults: Record<string, string>
  connection: 'checking' | 'offline_preview' | 'ready' | 'error'
  project?: ProjectInfo
  liveThreads: Thread[]
  items: Item[]
  input: string
  attachments: AttachmentRef[]
  busy: boolean
  activeTurn?: string
  experimentRun?: AELRunRecord
  releaseGates: Partial<Record<ReleaseResult['profile'], ReleaseResult>>
  backendError?: string
  setView: (view: WorkspaceView) => void
  setRunning: (running: boolean) => void
  setInspectorTab: (tab: WorkspaceState['inspectorTab']) => void
  decideApproval: (status: Approval['status']) => void
  connect: () => Promise<void>
  startDaemon: () => Promise<void>
  selectProject: () => Promise<void>
  setModel: (model: string) => void
  changePermission: (permission: 'read_only' | 'workspace_write' | 'full_access') => Promise<void>
  selectThread: (threadID: string) => Promise<void>
  setInput: (input: string) => void
  pickAttachments: () => Promise<void>
  removeAttachment: (sha256: string) => void
  submit: () => Promise<void>
  cancelTurn: () => Promise<void>
  resolveLiveApproval: (approvalID: string, allow: boolean) => Promise<void>
  startExperiment: () => Promise<void>
  cancelExperiment: () => Promise<void>
  checkRelease: () => Promise<void>
  spawnAgent: (prompt: string, permission?: 'read_only' | 'workspace_write') => Promise<void>
  messageAgent: (id: string, message: string, steer: boolean) => Promise<void>
  waitAgent: (id: string) => Promise<void>
  interruptAgent: (id: string) => Promise<void>
  closeAgent: (id: string) => Promise<void>
  handoffAgent: (id: string) => Promise<void>
  saveAutomation: (name: string, prompt: string, rrule: string, eventSource?: string) => Promise<void>
  toggleAutomation: (id: string) => Promise<void>
  triggerAutomations: (eventSource: string) => Promise<void>
  runAutomation: (id: string) => Promise<void>
  cancelAutomation: () => Promise<void>
  deleteAutomation: (id: string) => Promise<void>
  installPlugin: (approvePermissions: boolean) => Promise<void>
  revokePlugin: (id: string) => Promise<void>
  startReview: (scope: string, base: string) => Promise<void>
}

const sleep = (milliseconds: number) => new Promise((resolve) => window.setTimeout(resolve, milliseconds))

export const useWorkspace = create<WorkspaceState>((set, get) => ({
  view: 'chat',
  running: false,
  selectedThread: '',
  model: 'gpt-5.6',
  desiredPermission: 'workspace_write',
  inspectorTab: 'agents',
  approval: { id: '', title: '', description: '', resources: [], additions: 0, deletions: 0, status: 'denied' },
  threads: [],
  agents: [],
  liveAgents: [],
  automations: [],
  plugins: [],
  agentResults: {},
  handoffResults: {},
  connection: 'checking',
  liveThreads: [],
  items: [],
  input: '',
  attachments: [],
  busy: false,
  releaseGates: {},
  setView: (view) => set({
    view,
    inspectorTab: view === 'simulation' ? 'evidence' : view === 'browser' ? 'context' : 'agents',
  }),
  setRunning: (running) => set({ running }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
  decideApproval: (status) => set((state) => ({ approval: { ...state.approval, status } })),
  connect: async () => {
    const api = backend()
    if (!api) {
      set({ connection: 'offline_preview' })
      return
    }
    try {
      await api.Health()
      set({ connection: 'ready', backendError: undefined })
    } catch (error) {
      set({ connection: 'error', backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  startDaemon: async () => {
    const api = backend()
    if (!api) return
    set({ connection: 'checking', backendError: undefined })
    try {
      await api.InstallBackgroundService()
      for (let attempt = 0; attempt < 40; attempt += 1) {
        try {
          await api.Health()
          set({ connection: 'ready', backendError: undefined })
          return
        } catch {
          await sleep(250)
        }
      }
      throw new Error('后台服务启动超时，请检查“系统设置 → 通用 → 登录项”。')
    } catch (error) {
      set({ connection: 'error', backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  selectProject: async () => {
    const api = backend()
    if (!api) {
      set({ connection: 'offline_preview', backendError: '当前是浏览器离线预览；请从 Aether Desktop.app 打开项目。' })
      return
    }
    try {
      const project = await api.SelectProject(get().desiredPermission)
      const [liveThreads, liveAgents, automations, plugins] = await Promise.all([api.ListThreads(project.id), api.ListAgents(), api.ListAutomations(), api.ListPlugins()])
      const selectedThread = liveThreads[0]?.id ?? ''
      const items = selectedThread ? await api.Items(selectedThread, 0) : []
      set({ project, liveThreads, liveAgents, automations: automations.filter((automation) => automation.project_id === project.id), plugins, selectedThread, items, connection: 'ready', backendError: undefined })
      void get().checkRelease()
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  setModel: (model) => set({ model }),
  changePermission: async (permission) => {
    const api = backend()
    set({ desiredPermission: permission })
    if (!api || !get().project) return
    try {
      set({ project: await api.ChangeProjectPermission(permission), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  selectThread: async (threadID) => {
    const api = backend()
    if (!api) return
    try {
      const items = await api.Items(threadID, 0)
      set({ selectedThread: threadID, items, backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  setInput: (input) => set({ input }),
  pickAttachments: async () => {
    const api = backend()
    if (!api) return
    try {
      set({ attachments: await api.PickImageAttachments(), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  removeAttachment: (sha256) => set((state) => ({ attachments: state.attachments.filter((attachment) => attachment.sha256 !== sha256) })),
  submit: async () => {
    const api = backend()
    const state = get()
    const prompt = state.input.trim()
    if (!api || !state.project || !prompt || state.busy) return
    const attachments = state.attachments
    set({ busy: true, input: '', attachments: [], backendError: undefined })
    try {
      let thread = state.liveThreads.find((candidate) => candidate.id === state.selectedThread)
      if (!thread) {
        thread = await api.CreateThreadWithModel(state.project.id, prompt.slice(0, 48), state.project.permission, state.model.trim())
        set((current) => ({ liveThreads: [thread!, ...current.liveThreads], selectedThread: thread!.id }))
      }
      const turn = attachments.length > 0 ? await api.RunTurnWithAttachments(thread, prompt, attachments) : await api.RunTurn(thread, prompt)
      set({ activeTurn: turn.id })
      let after = state.items.at(-1)?.sequence ?? 0
      for (let attempt = 0; attempt < 480; attempt += 1) {
        const incoming = await api.Items(thread.id, after)
        if (incoming.length > 0) {
          after = incoming.at(-1)!.sequence
          set((current) => ({ items: [...current.items, ...incoming.filter((item) => !current.items.some((existing) => existing.id === item.id))] }))
        }
        const [threads, liveAgents] = await Promise.all([api.ListThreads(state.project.id), api.ListAgents()])
        const currentThread = threads.find((candidate) => candidate.id === thread!.id)
        set({ liveThreads: threads, liveAgents })
        if (currentThread && currentThread.status !== 'running' && currentThread.status !== 'waiting_for_approval') break
        await sleep(250)
      }
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    } finally {
      set({ busy: false, activeTurn: undefined })
    }
  },
  cancelTurn: async () => {
    const api = backend()
    const turnID = get().activeTurn
    if (!api || !turnID) return
    try {
      await api.CancelTurn(turnID)
      set({ busy: false, activeTurn: undefined, backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  resolveLiveApproval: async (approvalID, allow) => {
    const api = backend()
    if (!api) return
    try {
      await api.ResolveApproval(approvalID, allow)
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  startExperiment: async () => {
    const api = backend()
    const project = get().project
    if (!api || !project || get().running) return
    set({ view: 'simulation', running: true, backendError: undefined })
    try {
      let record = await api.StartExperiment({
        project_id: project.id,
        experiment_path: 'benchmarks/v2/experiments/24-antenna-cross-domain-fixed.yaml',
        system_path: 'benchmarks/v2/systems/five-domain-fixed.yaml',
        source_revision: 'aether-desktop',
      })
      set({ experimentRun: record })
      for (let attempt = 0; attempt < 2400; attempt += 1) {
        record = await api.GetExperiment(record.id)
        set({ experimentRun: record })
        if (record.status !== 'queued' && record.status !== 'running') break
        await sleep(250)
      }
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    } finally {
      set({ running: false })
    }
  },
  cancelExperiment: async () => {
    const api = backend()
    const run = get().experimentRun
    if (!api || !run) return
    try {
      await api.CancelExperiment(run.id)
    } finally {
      set({ running: false })
    }
  },
  checkRelease: async () => {
    const api = backend()
    if (!api || !get().project) return
    const profiles: ReleaseResult['profile'][] = ['foundation', 'desktop', 'agent', 'simulation', 'simulation-extensions', 'software', 'development-package', 'production']
    const values = await Promise.all(profiles.map(async (profile) => {
      try { return await api.CheckRelease(profile) } catch (error) { return { profile, passed: false, failures: [error instanceof Error ? error.message : String(error)], checked: [] } }
    }))
    set({ releaseGates: Object.fromEntries(values.map((value) => [value.profile, value])) })
  },
  spawnAgent: async (prompt, permission = 'read_only') => {
    const api = backend()
    const state = get()
    const parent = state.liveThreads.find((thread) => thread.id === state.selectedThread)
    if (!api || !parent || !prompt.trim()) return
    try {
      await api.SpawnAgent(parent, prompt.trim(), {
        name: `子Agent ${state.liveAgents.length + 1}`,
        role: '并行工程任务',
        model: parent.model,
        reasoning_effort: 'medium',
        permission,
        tools: permission === 'workspace_write' ? ['file', 'search', 'command', 'git'] : ['file', 'search'],
        max_concurrency: 1,
      })
      set({ liveAgents: await api.ListAgents(), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  messageAgent: async (id, message, steer) => {
    const api = backend()
    if (!api || !message.trim()) return
    try {
      if (steer) await api.SteerAgent(id, message.trim())
      else await api.MessageAgent(id, message.trim())
      set({ liveAgents: await api.ListAgents(), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  waitAgent: async (id) => {
    const api = backend()
    if (!api) return
    try {
      await api.WaitAgent(id)
      const result = await api.AgentResult(id)
      set((state) => ({ liveAgents: state.liveAgents.map((agent) => agent.id === id ? result.handle : agent), agentResults: { ...state.agentResults, [id]: result.summary }, backendError: undefined }))
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  interruptAgent: async (id) => {
    const api = backend()
    if (!api) return
    try {
      await api.InterruptAgent(id)
      set({ liveAgents: await api.ListAgents(), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  closeAgent: async (id) => {
    const api = backend()
    if (!api) return
    try {
      await api.CloseAgent(id)
      set((state) => ({ liveAgents: state.liveAgents.filter((agent) => agent.id !== id), agentResults: Object.fromEntries(Object.entries(state.agentResults).filter(([key]) => key !== id)), handoffResults: Object.fromEntries(Object.entries(state.handoffResults).filter(([key]) => key !== id)), backendError: undefined }))
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  handoffAgent: async (id) => {
    const api = backend()
    if (!api) return
    try {
      const result = await api.HandoffAgent(id, true)
      set((state) => ({ liveAgents: state.liveAgents.map((agent) => agent.id === id ? { ...agent, worktree: undefined } : agent), handoffResults: { ...state.handoffResults, [id]: `${result.paths.length}个文件 · ${result.patch_sha256.slice(0, 12)}…` }, backendError: undefined }))
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  saveAutomation: async (name, prompt, rrule, eventSource = '') => {
    const api = backend()
    const project = get().project
    if (!api || !project || !name.trim() || !prompt.trim() || (!rrule.trim() && !eventSource.trim())) return
    try {
      await api.SaveAutomation({ api_version: 'aether.desktop/v1', id: crypto.randomUUID(), name: name.trim(), prompt: prompt.trim(), rrule: rrule.trim(), event_source: eventSource.trim(), project_id: project.id, use_worktree: true, permission: 'workspace_write', enabled: true, stop_policy: { timeout_seconds: 7200 } })
      set({ automations: (await api.ListAutomations()).filter((automation) => automation.project_id === project.id), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  toggleAutomation: async (id) => {
    const api = backend()
    const project = get().project
    const spec = get().automations.find((automation) => automation.id === id)
    if (!api || !project || !spec) return
    try {
      await api.SaveAutomation({ ...spec, enabled: !spec.enabled })
      set({ automations: (await api.ListAutomations()).filter((automation) => automation.project_id === project.id), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  triggerAutomations: async (eventSource) => {
    const api = backend()
    if (!api || !eventSource.trim()) return
    try {
      const jobs = await api.TriggerAutomations(eventSource.trim())
      if (jobs[0]) set({ runningAutomationJob: jobs[0], automationJob: await api.AutomationJob(jobs[0]), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  runAutomation: async (id) => {
    const api = backend()
    if (!api) return
    try {
      const jobID = await api.RunAutomation(id)
      set({ runningAutomationJob: jobID, automationJob: await api.AutomationJob(jobID), backendError: undefined })
      for (let attempt = 0; attempt < 720; attempt += 1) {
        const job = await api.AutomationJob(jobID)
        set({ automationJob: job })
        if (!['queued', 'running', 'recovering', 'waiting_for_approval'].includes(job.status)) break
        await sleep(500)
      }
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  cancelAutomation: async () => {
    const api = backend()
    const jobID = get().runningAutomationJob
    if (!api || !jobID) return
    try {
      await api.CancelAutomation(jobID)
      for (let attempt = 0; attempt < 40; attempt += 1) {
        const job = await api.AutomationJob(jobID)
        set({ automationJob: job, backendError: undefined })
        if (!['queued', 'running', 'recovering', 'waiting_for_approval'].includes(job.status)) break
        await sleep(100)
      }
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  deleteAutomation: async (id) => {
    const api = backend()
    const project = get().project
    if (!api || !project) return
    try {
      await api.DeleteAutomation(id)
      set({ automations: (await api.ListAutomations()).filter((automation) => automation.project_id === project.id), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  installPlugin: async (approvePermissions) => {
    const api = backend()
    if (!api) return
    try {
      await api.SelectAndInstallPlugin(approvePermissions)
      set({ plugins: await api.ListPlugins(), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  revokePlugin: async (id) => {
    const api = backend()
    if (!api) return
    try {
      await api.RevokePlugin(id, '用户从Aether Desktop撤销')
      set({ plugins: await api.ListPlugins(), backendError: undefined })
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    }
  },
  startReview: async (scope, base) => {
    const api = backend()
    const project = get().project
    if (!api || !project || get().busy) return
    set({ busy: true, view: 'chat', backendError: undefined })
    try {
      const review = await api.StartCodeReview(scope, base)
      set({ selectedThread: review.id, liveThreads: await api.ListThreads(project.id), items: [] })
      let after = 0
      for (let attempt = 0; attempt < 480; attempt += 1) {
        const incoming = await api.Items(review.id, after)
        if (incoming.length > 0) {
          after = incoming.at(-1)!.sequence
          set((state) => ({ items: [...state.items, ...incoming] }))
        }
        const threads = await api.ListThreads(project.id)
        const current = threads.find((thread) => thread.id === review.id)
        set({ liveThreads: threads })
        if (current && current.status !== 'running' && current.status !== 'waiting_for_approval') break
        await sleep(250)
      }
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    } finally {
      set({ busy: false })
    }
  },
}))
