import { ChevronDown, Maximize2, Plus, Trash2 } from 'lucide-react'

export function TerminalPanel() {
  return (
    <section className="terminal-panel">
      <header><strong>终端</strong>{['bash', 'renode', 'pyocd', 'make'].map((tab, index) => <button className={index === 0 ? 'active' : ''} key={tab}>{tab}</button>)}<span className="terminal-spacer" /><Plus size={14} /><Maximize2 size={14} /><Trash2 size={14} /><ChevronDown size={14} /></header>
      <pre><b>dev@Aether</b>{` agentic-embedded-lab % make test-uart
正在构建 tests/uart_overrun_test.elf
正在 Renode 中运行…
[==========] 100%  全部测试通过（42.18 秒）
`}<b>dev@Aether</b>{` agentic-embedded-lab % `}<span className="cursor" /></pre>
    </section>
  )
}
