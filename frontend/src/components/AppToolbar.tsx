import { Box, GitBranch, Globe2, Play, Square } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

const views = ['chat', 'diff', 'browser', 'simulation'] as const

export function AppToolbar() {
  const view = useWorkspace((state) => state.view)
  const running = useWorkspace((state) => state.running)
  const setView = useWorkspace((state) => state.setView)
  const setRunning = useWorkspace((state) => state.setRunning)
  return (
    <header className="app-toolbar">
      <div className="traffic-lights" aria-hidden="true"><i /><i /><i /></div>
      <div className="brand">Aether</div>
      <div className="project-name">agentic-embedded-lab</div>
      <div className="branch-label"><GitBranch size={14} /> main</div>
      <nav className="view-tabs" aria-label="Workspace views">
        {views.map((item) => (
          <button key={item} className={view === item ? 'active' : ''} onClick={() => setView(item)}>
            {item[0].toUpperCase() + item.slice(1)}
          </button>
        ))}
      </nav>
      <div className="toolbar-actions">
        <button className="select-button">gpt-5.6 <span>⌄</span></button>
        <button className="select-button">Workspace Write <span>⌄</span></button>
        <button className="tool-button" onClick={() => setView('browser')}><Globe2 size={15} /> Browser</button>
        <button className={running ? 'run-button running' : 'run-button'} onClick={() => setRunning(!running)}>
          {running ? <Square size={14} /> : <Play size={15} />} {running ? 'Stop' : 'Run experiment'}
        </button>
        <button className="icon-button" aria-label="Open workspace menu"><Box size={16} /></button>
      </div>
    </header>
  )
}
