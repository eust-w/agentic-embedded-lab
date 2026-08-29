import { ChevronDown, Maximize2, Plus, Trash2 } from 'lucide-react'

export function TerminalPanel() {
  return (
    <section className="terminal-panel">
      <header><strong>Terminal</strong>{['bash', 'renode', 'pyocd', 'make'].map((tab, index) => <button className={index === 0 ? 'active' : ''} key={tab}>{tab}</button>)}<span className="terminal-spacer" /><Plus size={14} /><Maximize2 size={14} /><Trash2 size={14} /><ChevronDown size={14} /></header>
      <pre><b>dev@Aether</b>{` agentic-embedded-lab % make test-uart
Building tests/uart_overrun_test.elf
Running on Renode...
[==========] 100%  All tests passed (42.18s)
`}<b>dev@Aether</b>{` agentic-embedded-lab % `}<span className="cursor" /></pre>
    </section>
  )
}
