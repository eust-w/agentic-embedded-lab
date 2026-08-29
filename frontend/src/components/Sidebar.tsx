import { CalendarClock, ChevronDown, Cpu, Plus, Search } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

export function Sidebar() {
  const threads = useWorkspace((state) => state.threads)
  const selectedThread = useWorkspace((state) => state.selectedThread)
  return (
    <aside className="sidebar">
      <section className="sidebar-section worktree-section">
        <div className="section-label">WORKTREE</div>
        <button className="project-card">
          <span><strong>agentic-embedded-lab</strong><small>/Users/dev/agentic-embedded-lab</small></span>
          <span className="status-dot online" />
          <ChevronDown size={14} />
        </button>
      </section>
      <section className="sidebar-section thread-section">
        <label className="search-field"><Search size={14} /><input aria-label="Search threads" placeholder="Search threads..." /><kbd>⌘K</kbd></label>
        <div className="thread-list">
          {threads.map((thread) => (
            <button key={thread.id} className={`thread-row ${selectedThread === thread.id ? 'selected' : ''}`}>
              <span><strong>{thread.title}</strong><small>{thread.subtitle}</small></span><time>{thread.updated}</time>
            </button>
          ))}
        </div>
      </section>
      <section className="sidebar-section compact-section">
        <div className="section-label">SCHEDULED</div>
        <div className="utility-row"><CalendarClock size={14} /><span>Nightly HIL sweep</span><time>02:00</time></div>
        <div className="utility-row"><CalendarClock size={14} /><span>Weekly static analysis</span><time>Sun</time></div>
      </section>
      <section className="sidebar-section compact-section plugins-section">
        <div className="section-label">PLUGINS</div>
        {['STM32Cube', 'pyOCD', 'Renode', 'clang-tidy'].map((name, index) => (
          <div className="utility-row" key={name}><Cpu size={14} /><span>{name}</span><span className={`status-dot ${index < 3 ? 'online' : ''}`} /></div>
        ))}
        <button className="add-plugin"><Plus size={14} /> Add plugin</button>
      </section>
      <footer className="sidebar-footer"><span className="status-dot online" /> All systems operational</footer>
    </aside>
  )
}
