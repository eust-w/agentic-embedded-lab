import { AppToolbar } from './components/AppToolbar'
import { BrowserWorkspace } from './components/BrowserWorkspace'
import { ChatWorkspace } from './components/ChatWorkspace'
import { Inspector } from './components/Inspector'
import { Sidebar } from './components/Sidebar'
import { SimulationWorkspace } from './components/SimulationWorkspace'
import { TerminalPanel } from './components/TerminalPanel'
import { useWorkspace } from './store/workspace'

export function App() {
  const view = useWorkspace((state) => state.view)
  return (
    <div className="app-shell">
      <AppToolbar />
      <Sidebar />
      <main className="workspace-shell">
        {view === 'simulation' ? <SimulationWorkspace /> : view === 'browser' ? <BrowserWorkspace /> : <ChatWorkspace mode={view} />}
        {view !== 'browser' && view !== 'simulation' ? <TerminalPanel /> : null}
      </main>
      <Inspector />
    </div>
  )
}
