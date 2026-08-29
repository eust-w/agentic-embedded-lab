import { fireEvent, render, screen } from '@testing-library/react'
import { App } from './App'
import { useWorkspace } from './store/workspace'

beforeEach(() => {
  useWorkspace.setState((state) => ({
    ...state,
    view: 'chat',
    approval: { ...state.approval, status: 'pending' },
  }))
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
})
