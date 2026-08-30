import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { vi } from 'vitest'
import { App } from './App'
import { useWorkspace } from './store/workspace'
import type { BackendAPI } from './lib/backend'
import type { Item, Thread } from './types'

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    open() {}
    onData() { return { dispose() {} } }
    write() {}
    clear() {}
    dispose() {}
  },
}))

beforeEach(() => {
  useWorkspace.setState((state) => ({
    ...state,
    view: 'chat',
    model: 'gpt-5.6',
    desiredPermission: 'workspace_write',
    approval: { ...state.approval, status: 'denied' },
    project: undefined,
    liveThreads: [],
    liveAgents: [],
    automations: [],
    plugins: [],
    items: [],
    input: '',
    attachments: [],
    busy: false,
    activeTurn: undefined,
    agentResults: {},
    handoffResults: {},
    backendError: undefined,
  }))
  delete window.go
})

describe('Aether desktop shell', () => {
  it('switches between coding and simulation workspaces', () => {
    render(<App />)
    expect(screen.getByText('选择项目后开始真实工程任务')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '仿真' }))
    expect(screen.getByText('尚无真实仿真运行')).toBeInTheDocument()
    expect(screen.getByText(/不会展示演示波形/)).toBeInTheDocument()
  })

  it('does not render a fabricated approval before a real tool request', () => {
    render(<App />)
    expect(screen.queryByRole('button', { name: '仅批准本次' })).not.toBeInTheDocument()
    expect(screen.getByText(/不会展示演示任务/)).toBeInTheDocument()
  })

  it('shows an explicit Chinese blocker when bundled Chromium is unavailable', () => {
    render(<App />)
    fireEvent.click(within(screen.getByRole('navigation', { name: '工作区视图' })).getByRole('button', { name: '浏览器' }))
    expect(screen.getByText('受控 Chromium 尚未启动')).toBeInTheDocument()
    expect(screen.getByText(/不会静默改用系统浏览器/)).toBeInTheDocument()
  })

  it('does not show a fabricated diff before a Git project is selected', async () => {
    render(<App />)
    fireEvent.click(within(screen.getByRole('navigation', { name: '工作区视图' })).getByRole('button', { name: '变更' }))
    expect(await screen.findByText('选择 Git 项目后查看真实变更', {}, { timeout: 5000 })).toBeInTheDocument()
  })

  it('filters persisted tasks and focuses search with Command-K', () => {
    const now = new Date().toISOString()
    useWorkspace.setState({ project: { id: 'p', root: '/tmp/project', branch: 'main', permission: 'workspace_write', tools: [] }, liveThreads: [
      { api_version: 'aether.desktop/v1', id: 'uart', project_id: 'p', title: '修复 UART', model: 'gpt-5.6', permission: 'workspace_write', status: 'ready', created_at: now, updated_at: now },
      { api_version: 'aether.desktop/v1', id: 'spi', project_id: 'p', title: '检查 SPI', model: 'gpt-5.6', permission: 'workspace_write', status: 'ready', created_at: now, updated_at: now },
    ] })
    render(<App />)
    fireEvent.keyDown(window, { key: 'k', metaKey: true })
    const search = screen.getByRole('textbox', { name: '搜索任务' })
    expect(search).toHaveFocus()
    fireEvent.change(search, { target: { value: 'UART' } })
    expect(screen.getByText('修复 UART')).toBeInTheDocument()
    expect(screen.queryByText('检查 SPI')).not.toBeInTheDocument()
  })

  it('opens a real Wails project and renders persisted daemon items', async () => {
    const thread: Thread = { api_version: 'aether.desktop/v1', id: 'thread-1', project_id: 'project-1', title: '真实任务', model: 'gpt-5.6', permission: 'workspace_write', status: 'ready', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
    const items: Item[] = [
      { api_version: 'aether.desktop/v1', id: 'item-1', thread_id: thread.id, turn_id: 'turn-1', sequence: 1, type: 'user_message', payload: { text: '检查 UART' }, created_at: new Date().toISOString() },
      { api_version: 'aether.desktop/v1', id: 'item-2', thread_id: thread.id, turn_id: 'turn-1', sequence: 2, type: 'agent_message', payload: { delta: '已完成真实检查' }, created_at: new Date().toISOString() },
    ]
    const api: BackendAPI = {
      Health: vi.fn().mockResolvedValue({ status: 'ready', time: new Date().toISOString() }),
      InstallBackgroundService: vi.fn().mockResolvedValue(undefined),
      UpdateStatus: vi.fn().mockResolvedValue({ available: false, started: false }),
      SelectProject: vi.fn().mockResolvedValue({ id: 'project-1', root: '/tmp/firmware', branch: 'main', permission: 'workspace_write', tools: [] }),
      ChangeProjectPermission: vi.fn().mockResolvedValue({ id: 'project-1', root: '/tmp/firmware', branch: 'main', permission: 'read_only', tools: [] }),
      ListThreads: vi.fn().mockResolvedValueOnce([]).mockResolvedValue([thread]),
      CreateThread: vi.fn().mockResolvedValue(thread),
      CreateThreadWithModel: vi.fn().mockResolvedValue(thread),
      Items: vi.fn().mockResolvedValue(items),
      RunTurn: vi.fn().mockResolvedValue({ api_version: 'aether.desktop/v1', id: 'turn-1', thread_id: thread.id, status: 'running', input: '检查 UART', started_at: new Date().toISOString() }),
      RunTurnWithAttachments: vi.fn().mockResolvedValue({ api_version: 'aether.desktop/v1', id: 'turn-1', thread_id: thread.id, status: 'running', input: '检查 UART', started_at: new Date().toISOString() }),
      PickImageAttachments: vi.fn().mockResolvedValue([]),
      CancelTurn: vi.fn().mockResolvedValue(true),
      ResolveApproval: vi.fn().mockResolvedValue(true),
      SpawnAgent: vi.fn().mockResolvedValue({}),
      ListAgents: vi.fn().mockResolvedValue([]),
      MessageAgent: vi.fn().mockResolvedValue({}),
      SteerAgent: vi.fn().mockResolvedValue({}),
      WaitAgent: vi.fn().mockResolvedValue({}),
      AgentResult: vi.fn().mockResolvedValue({ handle: {}, items: [], summary: '' }),
      InterruptAgent: vi.fn().mockResolvedValue(true),
      CloseAgent: vi.fn().mockResolvedValue(true),
      HandoffAgent: vi.fn().mockResolvedValue({ patch_sha256: 'a'.repeat(64), paths: ['file.c'], applied: true, cleaned_up: true }),
      StartExperiment: vi.fn().mockResolvedValue({}),
      GetExperiment: vi.fn().mockResolvedValue({}),
      CancelExperiment: vi.fn().mockResolvedValue(true),
      CheckRelease: vi.fn().mockImplementation((profile) => Promise.resolve({ profile, passed: profile === 'foundation', failures: profile === 'foundation' ? [] : ['证据缺失'], checked: [] })),
      BrowserStatus: vi.fn().mockResolvedValue({ running: false, executable: '/Applications/Aether Desktop.app/Contents/Resources/Chromium.app/Contents/MacOS/Chromium' }),
      StartBrowser: vi.fn().mockResolvedValue(undefined),
      StopBrowser: vi.fn().mockResolvedValue(undefined),
      SetSitePermission: vi.fn().mockResolvedValue(undefined),
      RevokeSitePermission: vi.fn().mockResolvedValue(undefined),
      NavigateBrowser: vi.fn().mockResolvedValue(undefined),
      BrowserDOM: vi.fn().mockResolvedValue(''),
      BrowserScreenshot: vi.fn().mockResolvedValue(''),
      BrowserConsole: vi.fn().mockResolvedValue([]),
      BrowserNetwork: vi.fn().mockResolvedValue([]),
      LatestChromeSnapshot: vi.fn().mockResolvedValue({ available: false }),
      BrowserClick: vi.fn().mockResolvedValue(undefined),
      BrowserType: vi.fn().mockResolvedValue(undefined),
      BrowserDownload: vi.fn().mockResolvedValue('/tmp/downloads/file.bin'),
      ComputerStatus: vi.fn().mockResolvedValue({ accessibility: false, screen_recording: false }),
      ComputerDecision: vi.fn().mockResolvedValue({ bundle_id: 'com.google.Chrome', decision: 'ask' }),
      SetComputerPermission: vi.fn().mockResolvedValue(undefined),
      ComputerTree: vi.fn().mockResolvedValue('{}'),
      ComputerScreenshot: vi.fn().mockResolvedValue('data:image/png;base64,cG5n'),
      ComputerClick: vi.fn().mockResolvedValue(undefined),
      ComputerType: vi.fn().mockResolvedValue(undefined),
      StartTerminal: vi.fn().mockResolvedValue({ id: 'term-1', workspace: '/tmp/firmware', shell: '/bin/zsh -l', running: true, exit_code: -1, created_at: new Date().toISOString() }),
      ListTerminals: vi.fn().mockResolvedValue([]),
      ReadTerminal: vi.fn().mockResolvedValue({ id: 'term-1', workspace: '/tmp/firmware', shell: '/bin/zsh -l', running: true, exit_code: -1, created_at: new Date().toISOString(), offset: 0, next_offset: 0, data_base64: '', truncated: false }),
      WriteTerminal: vi.fn().mockResolvedValue(undefined),
      ResizeTerminal: vi.fn().mockResolvedValue(undefined),
      StopTerminal: vi.fn().mockResolvedValue(undefined),
      GitChanges: vi.fn().mockResolvedValue([]),
      GitFileContent: vi.fn().mockResolvedValue({ path: 'file.c', original: '', modified: '', language: 'c' }),
      GitStage: vi.fn().mockResolvedValue(undefined),
      GitUnstage: vi.fn().mockResolvedValue(undefined),
      GitRestore: vi.fn().mockResolvedValue(undefined),
      GitCommit: vi.fn().mockResolvedValue('0123456789abcdef'),
      GitPush: vi.fn().mockResolvedValue(undefined),
      GitCreatePullRequest: vi.fn().mockResolvedValue({ url: 'https://github.com/example/repo/pull/1' }),
      StartCodeReview: vi.fn().mockResolvedValue({}),
      SaveAutomation: vi.fn().mockImplementation((spec) => Promise.resolve(spec)),
      ListAutomations: vi.fn().mockResolvedValue([]),
      RunAutomation: vi.fn().mockResolvedValue('job-1'),
      TriggerAutomations: vi.fn().mockResolvedValue([]),
      CancelAutomation: vi.fn().mockResolvedValue(true),
      AutomationJob: vi.fn().mockResolvedValue({ id: 'job-1', automation_id: 'nightly', status: 'completed', attempt: 1, created_at: new Date().toISOString(), updated_at: new Date().toISOString() }),
      DeleteAutomation: vi.fn().mockResolvedValue(undefined),
      ListPlugins: vi.fn().mockResolvedValue([]),
      SelectAndInstallPlugin: vi.fn().mockResolvedValue({}),
      RevokePlugin: vi.fn().mockResolvedValue(undefined),
      MemoryStatus: vi.fn().mockResolvedValue({ global: false, project: false }),
      SetMemoryEnabled: vi.fn().mockResolvedValue(undefined),
      ListMemories: vi.fn().mockResolvedValue([]),
      SaveMemory: vi.fn().mockResolvedValue({}),
      DeleteMemory: vi.fn().mockResolvedValue(undefined),
    }
    window.go = { app: { Backend: api } }
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: /选择项目工作区/ }))
    const composer = await screen.findByPlaceholderText('描述要完成的嵌入式开发任务…')
    fireEvent.change(screen.getByRole('combobox', { name: '新任务模型' }), { target: { value: 'gpt-test-model' } })
    fireEvent.change(composer, { target: { value: '检查 UART' } })
    fireEvent.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(screen.getByText('已完成真实检查')).toBeInTheDocument())
    expect(api.RunTurn).toHaveBeenCalledTimes(1)
    expect(api.CreateThreadWithModel).toHaveBeenCalledWith('project-1', '检查 UART', 'workspace_write', 'gpt-test-model')
  })
})
