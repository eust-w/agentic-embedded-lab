import { useCallback, useEffect, useState } from 'react'
import { DiffEditor, loader } from '@monaco-editor/react'
import * as monaco from 'monaco-editor/editor/editor.api.js'
import EditorWorker from 'monaco-editor/editor/editor.worker.js?worker'
import 'monaco-editor/languages/definitions/cpp/register.js'
import 'monaco-editor/languages/definitions/go/register.js'
import 'monaco-editor/languages/definitions/python/register.js'
import 'monaco-editor/languages/definitions/rust/register.js'
import 'monaco-editor/languages/definitions/shell/register.js'
import 'monaco-editor/languages/definitions/yaml/register.js'
import { FileCode2, GitBranch, LoaderCircle, RefreshCw, RotateCcw, ScanSearch, Undo2 } from 'lucide-react'
import { backend } from '../lib/backend'
import { useWorkspace } from '../store/workspace'
import type { GitChange, GitFileContent } from '../types'

type DiffScope = 'unstaged' | 'staged' | 'branch' | 'commit'

loader.config({ monaco })
;(self as unknown as { MonacoEnvironment: { getWorker: () => Worker } }).MonacoEnvironment = { getWorker: () => new EditorWorker() }

const labels: Record<DiffScope, string> = { unstaged: '未暂存', staged: '已暂存', branch: '分支', commit: '提交' }

export function DiffWorkspace() {
  const project = useWorkspace((state) => state.project)
  const startReview = useWorkspace((state) => state.startReview)
  const [scope, setScope] = useState<DiffScope>('unstaged')
  const [base, setBase] = useState('main')
  const [changes, setChanges] = useState<GitChange[]>([])
  const [selected, setSelected] = useState('')
  const [content, setContent] = useState<GitFileContent>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    const api = backend()
    if (!api || !project) return
    setBusy(true)
    setError('')
    try {
      const next = await api.GitChanges(scope, scope === 'branch' || scope === 'commit' ? base : '')
      const nextSelected = next.some((change) => change.path === selected) ? selected : next[0]?.path ?? ''
      setChanges(next)
      setSelected(nextSelected)
      setContent(nextSelected ? await api.GitFileContent(nextSelected, scope, scope === 'branch' || scope === 'commit' ? base : '') : undefined)
    } catch (reason) {
      setError(String(reason))
      setChanges([])
      setContent(undefined)
    } finally {
      setBusy(false)
    }
  }, [base, project, scope, selected])

  useEffect(() => { void refresh() }, [refresh])

  const select = async (path: string) => {
    const api = backend()
    if (!api) return
    setSelected(path)
    setBusy(true)
    try {
      setContent(await api.GitFileContent(path, scope, scope === 'branch' || scope === 'commit' ? base : ''))
    } catch (reason) {
      setError(String(reason))
    } finally {
      setBusy(false)
    }
  }

  const mutate = async (action: 'stage' | 'unstage' | 'restore') => {
    const api = backend()
    if (!api || !selected) return
    if (action === 'restore' && !window.confirm(`确认放弃 ${selected} 的未暂存修改？此操作不会进入废纸篓。`)) return
    setBusy(true)
    try {
      if (action === 'stage') await api.GitStage([selected])
      else if (action === 'unstage') await api.GitUnstage([selected])
      else await api.GitRestore([selected])
      await refresh()
    } catch (reason) {
      setError(String(reason))
    } finally {
      setBusy(false)
    }
  }

  if (!project) return <section className="diff-workspace diff-empty"><GitBranch size={36}/><h2>选择 Git 项目后查看真实变更</h2><p>变更页不会使用演示 Diff，也不会在未确认时丢弃工作区修改。</p></section>

  return <section className="diff-workspace">
    <header className="diff-toolbar">
      <GitBranch size={16}/>
      {Object.entries(labels).map(([value, label]) => <button className={scope === value ? 'active' : ''} key={value} onClick={() => { setScope(value as DiffScope); setSelected('') }}>{label}</button>)}
      {scope === 'branch' || scope === 'commit' ? <input aria-label={scope === 'branch' ? '基准分支' : '提交 SHA'} value={base} onChange={(event) => setBase(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void refresh() }}/> : null}
      <button aria-label="刷新 Git 变更" onClick={() => void refresh()} disabled={busy}><RefreshCw className={busy ? 'spin' : ''} size={14}/></button>
      <button onClick={() => void startReview(scope, scope === 'branch' || scope === 'commit' ? base : '')} disabled={busy || changes.length === 0}><ScanSearch size={14}/>只读AI审查</button>
      <span>{changes.length} 个文件</span>
    </header>
    {error ? <div className="backend-error">{error}</div> : null}
    <div className="diff-layout">
      <aside className="diff-files">
        {changes.length === 0 ? <p>当前范围没有文本变更。</p> : changes.map((change) => <button className={selected === change.path ? 'active' : ''} key={change.path} onClick={() => void select(change.path)}><FileCode2 size={13}/><span>{change.path}</span><code>{change.index}{change.worktree}</code></button>)}
      </aside>
      <main className="monaco-diff-shell">
        <header><strong>{selected || '未选择文件'}</strong><span>{labels[scope]}</span>{scope === 'unstaged' && selected ? <><button onClick={() => void mutate('stage')} disabled={busy}><RotateCcw size={13}/>暂存</button><button className="danger" onClick={() => void mutate('restore')} disabled={busy}><Undo2 size={13}/>放弃修改</button></> : null}{scope === 'staged' && selected ? <button onClick={() => void mutate('unstage')} disabled={busy}><Undo2 size={13}/>取消暂存</button> : null}</header>
        {busy && !content ? <div className="diff-loading"><LoaderCircle className="spin"/>正在读取 Git 对象…</div> : content ? <DiffEditor height="100%" language={content.language} original={content.original} modified={content.modified} theme="vs-dark" options={{ readOnly: true, renderSideBySide: true, minimap: { enabled: false }, fontSize: 12, automaticLayout: true, scrollBeyondLastLine: false }}/> : <div className="diff-loading">选择左侧文件查看 Diff。</div>}
      </main>
    </div>
  </section>
}
