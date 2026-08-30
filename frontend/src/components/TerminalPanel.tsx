import { useEffect, useRef, useState } from 'react'
import { Maximize2, Plus, Square, Trash2 } from 'lucide-react'
import { Terminal as XTerm } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { backend } from '../lib/backend'
import { useWorkspace } from '../store/workspace'
import type { TerminalInfo } from '../types'

const encoder = new TextEncoder()

function bytesToBase64(bytes: Uint8Array) {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return window.btoa(binary)
}

function base64ToBytes(value: string) {
  const binary = window.atob(value)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index)
  return bytes
}

export function TerminalPanel() {
  const project = useWorkspace((state) => state.project)
  const container = useRef<HTMLDivElement>(null)
  const terminal = useRef<XTerm | null>(null)
  const session = useRef<TerminalInfo | null>(null)
  const [status, setStatus] = useState('选择项目后启动真实终端')
  const [terminalEpoch, setTerminalEpoch] = useState(0)
  const [expanded, setExpanded] = useState(false)

  useEffect(() => {
    const api = backend()
    const element = container.current
    if (!api || !project || !element) return
    let active = true
    let timer = 0
    let offset = 0
    const xterm = new XTerm({
      allowProposedApi: false,
      cursorBlink: true,
      convertEol: true,
      fontFamily: 'SFMono-Regular, Menlo, Monaco, monospace',
      fontSize: 11,
      lineHeight: 1.35,
      scrollback: 10_000,
      theme: { background: '#090f15', foreground: '#b9c4d0', cursor: '#dce7f2', green: '#4fd788', blue: '#58a8ff' },
    })
    terminal.current = xterm
    xterm.open(element)

    const dimensions = () => ({
      columns: Math.max(20, Math.min(240, Math.floor(element.clientWidth / 7.2))),
      rows: Math.max(5, Math.min(80, Math.floor(element.clientHeight / 18))),
    })

    const poll = async () => {
      if (!active || !session.current) return
      try {
        const snapshot = await api.ReadTerminal(session.current.id, offset)
        if (snapshot.truncated) xterm.write('\r\n\x1b[33m[Aether：早期终端输出已截断]\x1b[0m\r\n')
        if (snapshot.data_base64) xterm.write(base64ToBytes(snapshot.data_base64))
        offset = snapshot.next_offset
        session.current = snapshot
        setStatus(snapshot.running ? snapshot.shell : `进程已退出（${snapshot.exit_code}）`)
      } catch (reason) {
        if (active) setStatus(`终端读取失败：${String(reason)}`)
      }
      if (active && session.current?.running) timer = window.setTimeout(() => void poll(), 100)
    }

    const initialize = async () => {
      try {
        const existing = terminalEpoch === 0 ? (await api.ListTerminals()).find((value) => value.running && value.workspace === project.root) : undefined
        const size = dimensions()
        session.current = existing ?? await api.StartTerminal(size.columns, size.rows)
        setStatus(session.current.shell)
        await poll()
      } catch (reason) {
        setStatus(`终端启动失败：${String(reason)}`)
      }
    }

    const input = xterm.onData((data) => {
      if (session.current?.running) void api.WriteTerminal(session.current.id, bytesToBase64(encoder.encode(data)))
    })
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(() => {
      const size = dimensions()
      if (session.current?.running) void api.ResizeTerminal(session.current.id, size.columns, size.rows)
    })
    observer?.observe(element)
    void initialize()
    return () => {
      active = false
      window.clearTimeout(timer)
      observer?.disconnect()
      input.dispose()
      xterm.dispose()
      terminal.current = null
    }
  }, [project, terminalEpoch])

  const stop = async () => {
    const api = backend()
    if (api && session.current?.running) {
      await api.StopTerminal(session.current.id)
      setStatus('终端已停止')
    }
  }

  return <section className={`terminal-panel live-terminal-panel ${expanded ? 'expanded' : ''}`}>
    <header><strong>终端</strong><span className="active terminal-shell-label">zsh</span><span className="terminal-status">{status}</span><span className="terminal-spacer"/><button aria-label="新建终端" disabled={!project} onClick={() => setTerminalEpoch((value) => value + 1)}><Plus size={14}/></button><button aria-label={expanded ? '还原终端' : '最大化终端'} onClick={() => setExpanded((value) => !value)}><Maximize2 size={14}/></button><button aria-label="清空终端" onClick={() => terminal.current?.clear()}><Trash2 size={14}/></button><button aria-label="停止终端" onClick={() => void stop()}><Square size={13}/></button></header>
    <div className="xterm-host" ref={container}/>
  </section>
}
