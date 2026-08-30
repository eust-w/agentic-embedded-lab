import { lazy, Suspense, useEffect } from 'react'
import { AppToolbar } from './components/AppToolbar'
import { BrowserWorkspace } from './components/BrowserWorkspace'
import { ChatWorkspace } from './components/ChatWorkspace'
import { Inspector } from './components/Inspector'
import { Sidebar } from './components/Sidebar'
import { SimulationWorkspace } from './components/SimulationWorkspace'
import { useWorkspace } from './store/workspace'

const TerminalPanel = lazy(() => import('./components/TerminalPanel').then((module) => ({ default: module.TerminalPanel })))
const DiffWorkspace = lazy(() => import('./components/DiffWorkspace').then((module) => ({ default: module.DiffWorkspace })))

export function App() {
  const view = useWorkspace((state) => state.view)
  const connect = useWorkspace((state) => state.connect)
  const project = useWorkspace((state) => state.project)
  useEffect(() => { void connect() }, [connect])
  return (
    <div className="app-shell">
      <AppToolbar />
      <Sidebar />
      <main className="workspace-shell">
        {view === 'simulation' ? <SimulationWorkspace /> : view === 'browser' ? <BrowserWorkspace /> : view === 'diff' ? project ? <Suspense fallback={<section className="diff-workspace diff-empty">正在加载变更查看器…</section>}><DiffWorkspace /></Suspense> : <section className="diff-workspace diff-empty"><h2>选择 Git 项目后查看真实变更</h2><p>变更页不会使用演示 Diff，也不会在未确认时丢弃工作区修改。</p></section> : <ChatWorkspace mode={view} />}
        {view !== 'browser' && view !== 'simulation' ? <Suspense fallback={<section className="terminal-panel terminal-loading">正在加载终端…</section>}><TerminalPanel /></Suspense> : null}
      </main>
      <Inspector />
    </div>
  )
}
