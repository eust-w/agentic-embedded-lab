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
    title: 'Modify files in workspace',
    description: 'Apply the UART RX buffering fix in the active worktree.',
    resources: ['src/uart.c', 'src/uart_isr.c', 'include/uart.h', 'tests/uart_overrun_test.c'],
    additions: 42,
    deletions: 17,
    status: 'pending',
  },
  threads: [
    { id: 'uart-overrun', title: 'Fix UART timing overrun', subtitle: 'Timing drift at 115200 baud', updated: '2m ago' },
    { id: 'spi-dma', title: 'SPI DMA throughput drop', subtitle: 'Investigate underruns', updated: '1h ago' },
    { id: 'power-race', title: 'Power mode entry race', subtitle: 'WFI not entered', updated: '3h ago' },
    { id: 'can-recovery', title: 'CAN bus error recovery', subtitle: 'Bus-off recovery flaps', updated: '1d ago' },
    { id: 'itm-trace', title: 'Add ITM trace markers', subtitle: 'Trace channel 1', updated: '2d ago' },
  ],
  agents: [
    { id: 'coder', name: 'Coder', role: 'Implementation', status: 'working', progress: 74, detail: 'Inspecting UART ISR and ring buffer', worktree: 'feature/uart-fix', tone: 'blue' },
    { id: 'sim', name: 'Sim Engineer', role: 'Renode', status: 'running', progress: 52, detail: 'Running uart_overrun_high_load.repl', worktree: 'feature/uart-fix', tone: 'violet' },
    { id: 'test', name: 'Test Engineer', role: 'Verification', status: 'idle', progress: 0, detail: 'Waiting for changes', worktree: 'feature/uart-fix', tone: 'green' },
  ],
  setView: (view) => set({
    view,
    inspectorTab: view === 'simulation' ? 'evidence' : view === 'browser' ? 'context' : 'agents',
  }),
  setRunning: (running) => set({ running }),
  setInspectorTab: (inspectorTab) => set({ inspectorTab }),
  decideApproval: (status) => set((state) => ({ approval: { ...state.approval, status } })),
}))
