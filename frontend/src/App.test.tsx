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
    expect(screen.getByText('Fixing embedded firmware timing issue')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Simulation' }))
    expect(screen.getByText('Experiment timeline')).toBeInTheDocument()
    expect(screen.getByText('Hardware unverified')).toBeInTheDocument()
  })

  it('records an approval decision in local UI state', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'Approve once' }))
    expect(screen.getByText('Approved for this turn')).toBeInTheDocument()
  })
})
