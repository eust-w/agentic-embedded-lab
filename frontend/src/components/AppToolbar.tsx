import { Box, GitBranch, Globe2, Play, Square } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

const views = ['chat', 'diff', 'browser', 'simulation'] as const
const viewLabels = { chat: '对话', diff: '变更', browser: '浏览器', simulation: '仿真' }

export function AppToolbar() {
  const view = useWorkspace((state) => state.view)
  const running = useWorkspace((state) => state.running)
  const busy = useWorkspace((state) => state.busy)
  const setView = useWorkspace((state) => state.setView)
  const project = useWorkspace((state) => state.project)
  const selectProject = useWorkspace((state) => state.selectProject)
  const model = useWorkspace((state) => state.model)
  const setModel = useWorkspace((state) => state.setModel)
  const desiredPermission = useWorkspace((state) => state.desiredPermission)
  const changePermission = useWorkspace((state) => state.changePermission)
  const startExperiment = useWorkspace((state) => state.startExperiment)
  const cancelExperiment = useWorkspace((state) => state.cancelExperiment)
  return (
    <header className="app-toolbar">
      <div className="traffic-lights" aria-hidden="true"><i /><i /><i /></div>
      <div className="brand">Aether</div>
      <div className="project-name">{project?.root.split('/').at(-1) ?? '未选择项目'}</div>
      <div className="branch-label"><GitBranch size={14} /> {project?.branch ?? '无分支'}</div>
      <nav className="view-tabs" aria-label="工作区视图">
        {views.map((item) => (
          <button key={item} className={view === item ? 'active' : ''} onClick={() => setView(item)}>
            {viewLabels[item]}
          </button>
        ))}
      </nav>
      <div className="toolbar-actions">
        <input disabled={busy} className="select-button model-input" aria-label="新任务模型" value={model} onChange={(event) => setModel(event.target.value)} list="aether-models"/><datalist id="aether-models"><option value="gpt-5.6"/><option value="gpt-5.6-mini"/></datalist>
        <select disabled={busy} className="select-button" aria-label="项目权限" value={project?.permission ?? desiredPermission} onChange={(event) => void changePermission(event.target.value as 'read_only' | 'workspace_write' | 'full_access')}><option value="read_only">只读</option><option value="workspace_write">工作区写入</option><option value="full_access">完全访问</option></select>
        <button className="tool-button" onClick={() => setView('browser')}><Globe2 size={15} /> 浏览器</button>
        <button disabled={!project} className={running ? 'run-button running' : 'run-button'} onClick={() => project ? (running ? void cancelExperiment() : void startExperiment()) : undefined}>
          {running ? <Square size={14} /> : <Play size={15} />} {running ? '停止' : '运行实验'}
        </button>
        <button className="icon-button" aria-label="选择工作区" onClick={() => void selectProject()}><Box size={16} /></button>
      </div>
    </header>
  )
}
