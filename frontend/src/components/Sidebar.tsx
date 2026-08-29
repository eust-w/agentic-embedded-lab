import { CalendarClock, ChevronDown, Cpu, Plus, Search } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

export function Sidebar() {
  const threads = useWorkspace((state) => state.threads)
  const selectedThread = useWorkspace((state) => state.selectedThread)
  return (
    <aside className="sidebar">
      <section className="sidebar-section worktree-section">
        <div className="section-label">工作树</div>
        <button className="project-card">
          <span><strong>agentic-embedded-lab</strong><small>/Users/dev/agentic-embedded-lab</small></span>
          <span className="status-dot online" />
          <ChevronDown size={14} />
        </button>
      </section>
      <section className="sidebar-section thread-section">
        <label className="search-field"><Search size={14} /><input aria-label="搜索任务" placeholder="搜索任务…" /><kbd>⌘K</kbd></label>
        <div className="thread-list">
          {threads.map((thread) => (
            <button key={thread.id} className={`thread-row ${selectedThread === thread.id ? 'selected' : ''}`}>
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
      <footer className="sidebar-footer"><span className="status-dot online" /> 所有系统运行正常</footer>
    </aside>
  )
}
