import { CalendarClock, ChevronDown, Cpu, Plus, Search } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

export function Sidebar() {
  const threads = useWorkspace((state) => state.threads)
  const selectedThread = useWorkspace((state) => state.selectedThread)
  const project = useWorkspace((state) => state.project)
  const liveThreads = useWorkspace((state) => state.liveThreads)
  const connection = useWorkspace((state) => state.connection)
  const backendError = useWorkspace((state) => state.backendError)
  const selectProject = useWorkspace((state) => state.selectProject)
  const startDaemon = useWorkspace((state) => state.startDaemon)
  const selectThread = useWorkspace((state) => state.selectThread)
  const displayedThreads = project ? liveThreads.map((thread) => ({ id: thread.id, title: thread.title, subtitle: thread.status === 'running' ? '正在运行' : thread.status === 'failed' ? '运行失败' : thread.model, updated: new Date(thread.updated_at).toLocaleString('zh-CN') })) : threads
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
        <label className="search-field"><Search size={14} /><input aria-label="搜索任务" placeholder="搜索任务…" /><kbd>⌘K</kbd></label>
        <div className="thread-list">
          {displayedThreads.map((thread) => (
            <button key={thread.id} className={`thread-row ${selectedThread === thread.id ? 'selected' : ''}`} onClick={() => project ? void selectThread(thread.id) : undefined}>
              <span><strong>{thread.title}</strong><small>{thread.subtitle}</small></span><time>{thread.updated}</time>
            </button>
          ))}
        </div>
      </section>
      <section className="sidebar-section compact-section">
        <div className="section-label">自动化任务</div>
        <div className="utility-row"><CalendarClock size={14} /><span>夜间 HIL 扫描</span><time>02:00</time></div>
        <div className="utility-row"><CalendarClock size={14} /><span>每周静态分析</span><time>周日</time></div>
      </section>
      <section className="sidebar-section compact-section plugins-section">
        <div className="section-label">插件</div>
        {['STM32Cube', 'pyOCD', 'Renode', 'clang-tidy'].map((name, index) => (
          <div className="utility-row" key={name}><Cpu size={14} /><span>{name}</span><span className={`status-dot ${index < 3 ? 'online' : ''}`} /></div>
        ))}
        <button className="add-plugin"><Plus size={14} /> 添加插件</button>
      </section>
      <footer className="sidebar-footer" title={backendError}><span className={`status-dot ${connection === 'ready' ? 'online' : ''}`} /> {backendError ? '后台连接异常' : connection === 'ready' ? 'Aether 后台已连接' : '离线预览模式'}</footer>
    </aside>
  )
}
