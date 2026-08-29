import { create } from 'zustand'
import type { AgentStatus, Approval, ThreadSummary, WorkspaceView } from '../types'

interface WorkspaceState {
  view: WorkspaceView
  running: boolean
  selectedThread: string
  inspectorTab: 'context' | 'agents' | 'evidence'
  approval: Approval
  threads: ThreadSummary[]
  agents: AgentStatus[]
  setView: (view: WorkspaceView) => void
  setRunning: (running: boolean) => void
  setInspectorTab: (tab: WorkspaceState['inspectorTab']) => void
  decideApproval: (status: Approval['status']) => void
}

export const useWorkspace = create<WorkspaceState>((set) => ({
  view: 'chat',
  running: true,
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
  setView: (view) => set({
    view,
    inspectorTab: view === 'simulation' ? 'evidence' : view === 'browser' ? 'context' : 'agents',
  }),
  setRunning: (running) => set({ running }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
  decideApproval: (status) => set((state) => ({ approval: { ...state.approval, status } })),
}))
