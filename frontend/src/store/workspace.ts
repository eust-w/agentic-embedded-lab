import { create } from 'zustand'
import { backend } from '../lib/backend'
import type { AELRunRecord, AgentStatus, Approval, Item, ProjectInfo, ReleaseResult, Thread, ThreadSummary, WorkspaceView } from '../types'

interface WorkspaceState {
  view: WorkspaceView
  running: boolean
  selectedThread: string
  inspectorTab: 'context' | 'agents' | 'evidence'
  approval: Approval
  threads: ThreadSummary[]
  agents: AgentStatus[]
  connection: 'checking' | 'offline_preview' | 'ready' | 'error'
  project?: ProjectInfo
  liveThreads: Thread[]
  items: Item[]
  input: string
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
  selectThread: (threadID: string) => Promise<void>
  setInput: (input: string) => void
  submit: () => Promise<void>
  resolveLiveApproval: (approvalID: string, allow: boolean) => Promise<void>
  startExperiment: () => Promise<void>
  cancelExperiment: () => Promise<void>
  checkRelease: () => Promise<void>
}

const sleep = (milliseconds: number) => new Promise((resolve) => window.setTimeout(resolve, milliseconds))

export const useWorkspace = create<WorkspaceState>((set, get) => ({
  view: 'chat',
  running: false,
  selectedThread: 'uart-overrun',
  inspectorTab: 'agents',
  approval: {
    id: 'approval-1',
    title: '修改工作区文件',
    description: '在当前工作树中应用 UART RX 缓冲修复。',
    resources: ['src/uart.c', 'src/uart_isr.c', 'include/uart.h', 'tests/uart_overrun_test.c'],
    additions: 42,
    deletions: 17,
    status: 'pending',
  },
  threads: [
    { id: 'uart-overrun', title: '修复 UART 时序溢出', subtitle: '115200 波特率下发生时序漂移', updated: '2 分钟前' },
    { id: 'spi-dma', title: 'SPI DMA 吞吐下降', subtitle: '排查数据下溢', updated: '1 小时前' },
    { id: 'power-race', title: '电源模式进入竞争', subtitle: '未能进入 WFI', updated: '3 小时前' },
    { id: 'can-recovery', title: 'CAN 总线错误恢复', subtitle: 'Bus-off 恢复反复抖动', updated: '1 天前' },
    { id: 'itm-trace', title: '添加 ITM 追踪标记', subtitle: '追踪通道 1', updated: '2 天前' },
  ],
  agents: [
    { id: 'coder', name: '编码 Agent', role: '实现', status: 'working', progress: 74, detail: '正在检查 UART ISR 和环形缓冲区', worktree: 'feature/uart-fix', tone: 'blue' },
    { id: 'sim', name: '仿真 Agent', role: 'Renode', status: 'running', progress: 52, detail: '正在运行 uart_overrun_high_load.repl', worktree: 'feature/uart-fix', tone: 'violet' },
    { id: 'test', name: '测试 Agent', role: '验证', status: 'idle', progress: 0, detail: '正在等待变更', worktree: 'feature/uart-fix', tone: 'green' },
  ],
  connection: 'checking',
  liveThreads: [],
  items: [],
  input: '',
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
      const project = await api.SelectProject('workspace_write')
      const liveThreads = await api.ListThreads(project.id)
      const selectedThread = liveThreads[0]?.id ?? ''
      const items = selectedThread ? await api.Items(selectedThread, 0) : []
      set({ project, liveThreads, selectedThread, items, connection: 'ready', backendError: undefined })
      void get().checkRelease()
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
  submit: async () => {
    const api = backend()
    const state = get()
    const prompt = state.input.trim()
    if (!api || !state.project || !prompt || state.busy) return
    set({ busy: true, input: '', backendError: undefined })
    try {
      let thread = state.liveThreads.find((candidate) => candidate.id === state.selectedThread)
      if (!thread) {
        thread = await api.CreateThread(state.project.id, prompt.slice(0, 48), state.project.permission)
        set((current) => ({ liveThreads: [thread!, ...current.liveThreads], selectedThread: thread!.id }))
      }
      const turn = await api.RunTurn(thread, prompt)
      set({ activeTurn: turn.id })
      let after = state.items.at(-1)?.sequence ?? 0
      for (let attempt = 0; attempt < 480; attempt += 1) {
        const incoming = await api.Items(thread.id, after)
        if (incoming.length > 0) {
          after = incoming.at(-1)!.sequence
          set((current) => ({ items: [...current.items, ...incoming.filter((item) => !current.items.some((existing) => existing.id === item.id))] }))
        }
        const threads = await api.ListThreads(state.project.id)
        const currentThread = threads.find((candidate) => candidate.id === thread!.id)
        set({ liveThreads: threads })
        if (currentThread && currentThread.status !== 'running' && currentThread.status !== 'waiting_for_approval') break
        await sleep(250)
      }
    } catch (error) {
      set({ backendError: error instanceof Error ? error.message : String(error) })
    } finally {
      set({ busy: false, activeTurn: undefined })
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
    const profiles: ReleaseResult['profile'][] = ['foundation', 'simulation', 'software', 'production']
    const values = await Promise.all(profiles.map(async (profile) => {
      try { return await api.CheckRelease(profile) } catch (error) { return { profile, passed: false, failures: [error instanceof Error ? error.message : String(error)], checked: [] } }
    }))
    set({ releaseGates: Object.fromEntries(values.map((value) => [value.profile, value])) })
  },
}))
