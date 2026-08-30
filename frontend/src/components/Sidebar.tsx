import { useEffect, useRef, useState } from 'react'
import { CalendarClock, ChevronDown, Cpu, Play, Plus, Search, Square, Trash2 } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

export function Sidebar() {
  const selectedThread = useWorkspace((state) => state.selectedThread)
  const project = useWorkspace((state) => state.project)
  const liveThreads = useWorkspace((state) => state.liveThreads)
  const connection = useWorkspace((state) => state.connection)
  const backendError = useWorkspace((state) => state.backendError)
  const selectProject = useWorkspace((state) => state.selectProject)
  const startDaemon = useWorkspace((state) => state.startDaemon)
  const selectThread = useWorkspace((state) => state.selectThread)
  const automations = useWorkspace((state) => state.automations)
  const saveAutomation = useWorkspace((state) => state.saveAutomation)
  const toggleAutomation = useWorkspace((state) => state.toggleAutomation)
  const triggerAutomations = useWorkspace((state) => state.triggerAutomations)
  const runAutomation = useWorkspace((state) => state.runAutomation)
  const cancelAutomation = useWorkspace((state) => state.cancelAutomation)
  const deleteAutomation = useWorkspace((state) => state.deleteAutomation)
  const automationJob = useWorkspace((state) => state.automationJob)
  const plugins = useWorkspace((state) => state.plugins)
  const installPlugin = useWorkspace((state) => state.installPlugin)
  const revokePlugin = useWorkspace((state) => state.revokePlugin)
  const [showAutomation, setShowAutomation] = useState(false)
  const [automationName, setAutomationName] = useState('夜间嵌入式回归')
  const [automationPrompt, setAutomationPrompt] = useState('运行项目测试和AEL快速回归，分析失败并生成证据摘要。')
  const [automationRRULE, setAutomationRRULE] = useState('FREQ=DAILY;BYHOUR=2;BYMINUTE=0')
  const [automationEvent, setAutomationEvent] = useState('')
  const [query, setQuery] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        searchRef.current?.focus()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])
  const normalizedQuery = query.trim().toLocaleLowerCase('zh-CN')
  const displayedThreads = project ? liveThreads.filter((thread) => !normalizedQuery || `${thread.title} ${thread.status} ${thread.model}`.toLocaleLowerCase('zh-CN').includes(normalizedQuery)).map((thread) => ({ id: thread.id, title: thread.title, subtitle: thread.status === 'running' ? '正在运行' : thread.status === 'failed' ? '运行失败' : thread.model, updated: new Date(thread.updated_at).toLocaleString('zh-CN') })) : []
  return (
    <aside className="sidebar">
      <section className="sidebar-section worktree-section">
        <div className="section-label">工作树</div>
        <button className="project-card" onClick={() => connection === 'error' ? void startDaemon() : void selectProject()}>
          <span><strong>{project ? project.root.split('/').at(-1) : connection === 'error' ? '启动 Aether 后台' : '选择项目工作区'}</strong><small>{project?.root ?? (connection === 'offline_preview' ? '浏览器离线预览' : connection === 'error' ? '点击注册并启动登录用户服务' : '点击打开本地仓库')}</small></span>
          <span className={`status-dot ${connection === 'ready' ? 'online' : ''}`} />
          <ChevronDown size={14} />
        </button>
      </section>
      <section className="sidebar-section thread-section">
        <label className="search-field"><Search size={14} /><input ref={searchRef} aria-label="搜索任务" placeholder="搜索任务…" value={query} onChange={(event) => setQuery(event.target.value)} /><kbd>⌘K</kbd></label>
        <div className="thread-list">
          {displayedThreads.map((thread) => (
            <button key={thread.id} className={`thread-row ${selectedThread === thread.id ? 'selected' : ''}`} onClick={() => project ? void selectThread(thread.id) : undefined}>
              <span><strong>{thread.title}</strong><small>{thread.subtitle}</small></span><time>{thread.updated}</time>
            </button>
          ))}
          {displayedThreads.length === 0 ? <p className="sidebar-empty">{query ? '没有匹配任务。' : '尚无真实任务。'}</p> : null}
        </div>
      </section>
      <section className="sidebar-section compact-section">
        <div className="section-label">自动化任务 <button aria-label="新建自动化" disabled={!project} onClick={() => setShowAutomation((value) => !value)}><Plus size={12}/></button></div>
        {!project ? <p className="sidebar-empty">选择项目后显示真实RRULE任务。</p> : automations.length === 0 ? <p className="sidebar-empty">尚未创建自动化。</p> : automations.map((automation) => <div className="utility-row" key={automation.id}><CalendarClock size={14}/><span>{automation.name}<small>{automationJob?.automation_id === automation.id ? automationJob.status : automation.enabled ? automation.event_source ? `事件：${automation.event_source}` : '已启用' : '已停用'}</small></span><button aria-label={`${automation.enabled ? '停用' : '启用'}${automation.name}`} onClick={() => void toggleAutomation(automation.id)}>{automation.enabled ? 'Ⅱ' : '▶'}</button><button aria-label={`立即运行${automation.name}`} disabled={!automation.enabled || automationJob?.automation_id === automation.id && ['queued', 'running', 'recovering', 'waiting_for_approval'].includes(automationJob.status)} onClick={() => void runAutomation(automation.id)}><Play size={12}/></button>{automation.event_source ? <button aria-label={`触发${automation.event_source}`} disabled={!automation.enabled} onClick={() => void triggerAutomations(automation.event_source!)}>⚡</button> : null}{automationJob?.automation_id === automation.id && ['queued', 'running', 'recovering', 'waiting_for_approval'].includes(automationJob.status) ? <button aria-label={`取消${automation.name}`} onClick={() => void cancelAutomation()}><Square size={12}/></button> : <button aria-label={`删除${automation.name}`} onClick={() => void deleteAutomation(automation.id)}><Trash2 size={12}/></button>}</div>)}
        {showAutomation && project ? <form className="automation-form" onSubmit={(event) => { event.preventDefault(); void saveAutomation(automationName, automationPrompt, automationRRULE, automationEvent); setShowAutomation(false) }}><input aria-label="自动化名称" value={automationName} onChange={(event) => setAutomationName(event.target.value)}/><textarea aria-label="自动化任务" value={automationPrompt} onChange={(event) => setAutomationPrompt(event.target.value)}/><input aria-label="RRULE" value={automationRRULE} onChange={(event) => setAutomationRRULE(event.target.value)} placeholder="RRULE（与事件源至少填写一个）"/><input aria-label="插件事件源" value={automationEvent} onChange={(event) => setAutomationEvent(event.target.value)} placeholder="例如 plugin.model.updated"/><button className="primary-button" disabled={!automationRRULE.trim() && !automationEvent.trim()}>保存</button></form> : null}
      </section>
      <section className="sidebar-section compact-section plugins-section">
        <div className="section-label">插件</div>
        {plugins.length === 0 ? <p className="sidebar-empty">仅加载受信任Ed25519签名插件。</p> : plugins.map((plugin) => <div className="utility-row" key={plugin.manifest.id}><Cpu size={14}/><span>{plugin.manifest.name}<small> {plugin.manifest.version}</small></span><span className={`status-dot ${plugin.active && !plugin.revoked ? 'online' : ''}`}/>{plugin.active && !plugin.revoked ? <button aria-label={`撤销${plugin.manifest.name}`} onClick={() => void revokePlugin(plugin.manifest.id)}>×</button> : null}</div>)}
        <button className="add-plugin" disabled={!project} onClick={() => void installPlugin(false)}><Plus size={14} /> 添加签名插件</button>
        {backendError?.includes('additional permissions') ? <button className="add-plugin danger" onClick={() => void installPlugin(true)}>批准新增权限并升级</button> : null}
      </section>
      <footer className="sidebar-footer" title={backendError}><span className={`status-dot ${connection === 'ready' ? 'online' : ''}`} /> {backendError ? '后台连接异常' : connection === 'ready' ? 'Aether 后台已连接' : '后台尚未连接'}</footer>
    </aside>
  )
}
