import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import { App } from './App'
import { useWorkspace } from './store/workspace'
import type { BackendAPI } from './lib/backend'
import type { Item, Thread } from './types'

beforeEach(() => {
  useWorkspace.setState((state) => ({
    ...state,
    view: 'chat',
    approval: { ...state.approval, status: 'pending' },
    project: undefined,
    liveThreads: [],
    items: [],
    input: '',
    busy: false,
    backendError: undefined,
  }))
  delete window.go
})

describe('Aether desktop shell', () => {
  it('switches between coding and simulation workspaces', () => {
    render(<App />)
    expect(screen.getByText('正在修复嵌入式固件时序问题')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '仿真' }))
    expect(screen.getByText('实验时间线')).toBeInTheDocument()
    expect(screen.getByText('硬件尚未验证')).toBeInTheDocument()
  })

  it('records an approval decision in local UI state', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '仅批准本次' }))
    expect(screen.getByText('已批准当前轮次')).toBeInTheDocument()
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
      SelectProject: vi.fn().mockResolvedValue({ id: 'project-1', root: '/tmp/firmware', permission: 'workspace_write', tools: [] }),
      ListThreads: vi.fn().mockResolvedValueOnce([]).mockResolvedValue([thread]),
      CreateThread: vi.fn().mockResolvedValue(thread),
      Items: vi.fn().mockResolvedValue(items),
      RunTurn: vi.fn().mockResolvedValue({ api_version: 'aether.desktop/v1', id: 'turn-1', thread_id: thread.id, status: 'running', input: '检查 UART', started_at: new Date().toISOString() }),
      CancelTurn: vi.fn().mockResolvedValue(true),
      ResolveApproval: vi.fn().mockResolvedValue(true),
      SpawnAgent: vi.fn().mockResolvedValue({}),
      StartExperiment: vi.fn().mockResolvedValue({}),
      GetExperiment: vi.fn().mockResolvedValue({}),
      CancelExperiment: vi.fn().mockResolvedValue(true),
    }
    window.go = { app: { Backend: api } }
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: /选择项目工作区/ }))
    const composer = await screen.findByPlaceholderText('描述要完成的嵌入式开发任务…')
    fireEvent.change(composer, { target: { value: '检查 UART' } })
    fireEvent.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(screen.getByText('已完成真实检查')).toBeInTheDocument())
    expect(api.RunTurn).toHaveBeenCalledTimes(1)
  })
})
