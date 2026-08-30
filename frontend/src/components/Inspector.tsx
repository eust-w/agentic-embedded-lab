import { Activity, CheckCircle2, CircleAlert, CircleX, GitBranch, Globe2, MonitorCog, Settings, ShieldCheck, TextCursorInput } from 'lucide-react'
import { useWorkspace } from '../store/workspace'
import { backend } from '../lib/backend'
import type { AgentHandle } from '../types'
import type { AetherMemory } from '../types'

const tabLabels = { context: '上下文', agents: 'Agent', evidence: '证据' }

export function Inspector() {
  const tab = useWorkspace((state) => state.inspectorTab)
  const view = useWorkspace((state) => state.view)
  const setTab = useWorkspace((state) => state.setInspectorTab)
  const project = useWorkspace((state) => state.project)
  const liveAgents = useWorkspace((state) => state.liveAgents)
  return (
    <aside className="inspector">
      <nav className="inspector-tabs">
        {(['context', 'agents', 'evidence'] as const).map((item) => <button key={item} className={tab === item ? 'active' : ''} onClick={() => setTab(item)}>{tabLabels[item]}</button>)}
      </nav>
      {view === 'browser' ? <PermissionInspector /> : tab === 'agents' ? project ? <LiveAgentInspector agents={liveAgents}/> : <AgentInspector /> : tab === 'evidence' ? <EvidenceInspector /> : <ContextInspector />}
    </aside>
  )
}

function LiveAgentInspector({ agents }: { agents: AgentHandle[] }) {
  const [prompt, setPrompt] = useState('')
  const [writable, setWritable] = useState(false)
  const [messages, setMessages] = useState<Record<string, string>>({})
  const spawn = useWorkspace((state) => state.spawnAgent)
  const messageAgent = useWorkspace((state) => state.messageAgent)
  const waitAgent = useWorkspace((state) => state.waitAgent)
  const interrupt = useWorkspace((state) => state.interruptAgent)
  const closeAgent = useWorkspace((state) => state.closeAgent)
  const handoffAgent = useWorkspace((state) => state.handoffAgent)
  const results = useWorkspace((state) => state.agentResults)
  const handoffs = useWorkspace((state) => state.handoffResults)
  return <div className="inspector-content">
    <div className="inspector-heading">真实子Agent <span>{agents.length}</span></div>
    <form className="subagent-form" onSubmit={(event) => { event.preventDefault(); void spawn(prompt, writable ? 'workspace_write' : 'read_only'); setPrompt('') }}><textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder="描述可并行执行的子任务…"/><label className="sensitive-confirm"><input type="checkbox" checked={writable} onChange={(event) => setWritable(event.target.checked)}/>写入任务使用独立Worktree</label><button className="primary-button" disabled={!prompt.trim()}>启动子Agent</button></form>
    {agents.length === 0 ? <p className="empty-agent-state">尚未启动子Agent。写操作子Agent会使用独立Worktree；当前表单默认只读。</p> : agents.map((agent) => <article className="agent-card" key={agent.id}>
      <header><span className="agent-avatar blue">{agent.spec.name[0]}</span><div><strong>{agent.spec.name}</strong><small>{agent.status === 'active' ? '运行中' : agent.status === 'done' ? '已完成' : agent.status === 'failed' ? '失败' : '已中断'}</small></div><span className={`status-dot ${agent.status === 'active' ? 'online' : ''}`}/></header>
      <p>{agent.spec.role}</p><small>工作区</small><a>{agent.worktree?.branch || '只读项目快照'}</a><GitBranch size={13}/>
      <textarea aria-label={`发送给${agent.spec.name}`} value={messages[agent.id] ?? ''} onChange={(event) => setMessages((current) => ({ ...current, [agent.id]: event.target.value }))} placeholder="发送补充消息或转向指令…"/>
      <div className="agent-actions"><button disabled={!messages[agent.id]?.trim()} onClick={() => { void messageAgent(agent.id, messages[agent.id]!, false); setMessages((current) => ({ ...current, [agent.id]: '' })) }}>消息</button><button disabled={!messages[agent.id]?.trim()} onClick={() => { void messageAgent(agent.id, messages[agent.id]!, true); setMessages((current) => ({ ...current, [agent.id]: '' })) }}>转向</button>{agent.status === 'active' ? <><button onClick={() => void waitAgent(agent.id)}>等待结果</button><button className="secondary-button agent-stop" onClick={() => void interrupt(agent.id)}>中断</button></> : <button onClick={() => void waitAgent(agent.id)}>读取结果</button>}{agent.worktree && agent.status !== 'active' ? <button className="primary-button" onClick={() => void handoffAgent(agent.id)}>安全Handoff</button> : null}<button className="secondary-button" onClick={() => void closeAgent(agent.id)}>关闭</button></div>
      {results[agent.id] ? <pre className="agent-result">{results[agent.id]}</pre> : null}
      {handoffs[agent.id] ? <p className="handoff-result">已交回：{handoffs[agent.id]}</p> : null}
    </article>)}
    <EvidenceInspector compact />
  </div>
}

function PermissionInspector() {
  const [decision, setDecision] = useState<'ask' | 'allow' | 'deny'>('ask')
  const [native, setNative] = useState({ accessibility: false, screen_recording: false })
  const [error, setError] = useState('')
  const [tree, setTree] = useState('')
  const [screenshot, setScreenshot] = useState('')
  const [point, setPoint] = useState({ x: '0', y: '0' })
  const [text, setText] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const bundleID = 'com.google.Chrome'
  const refresh = async (prompt = false) => {
    const api = backend()
    if (!api) return
    try {
      const [nextNative, nextDecision] = await Promise.all([api.ComputerStatus(prompt), api.ComputerDecision(bundleID)])
      setNative(nextNative)
      setDecision(nextDecision.decision)
      setError('')
    } catch (reason) {
      setError(String(reason))
    }
  }
  useEffect(() => { void refresh() }, [])
  const decide = async (allow: boolean, scope: string) => {
    const api = backend()
    if (!api) return
    try {
      await api.SetComputerPermission(bundleID, allow, scope)
      await refresh()
    } catch (reason) {
      setError(String(reason))
    }
  }
  const operate = async (action: 'tree' | 'screenshot' | 'click' | 'type') => {
    const api = backend()
    if (!api) return
    setError('')
    try {
      if (action === 'tree') setTree(await api.ComputerTree(bundleID, 200))
      if (action === 'screenshot') setScreenshot(await api.ComputerScreenshot(bundleID))
      if (action === 'click') await api.ComputerClick(bundleID, Number(point.x), Number(point.y), confirmed)
      if (action === 'type') await api.ComputerType(bundleID, text, confirmed)
      await refresh()
    } catch (reason) {
      setError(String(reason))
    }
  }
  return <div className="inspector-content permission-content">
    <div className="inspector-heading">权限</div>
    <div className="permission-row"><ShieldCheck size={17}/><span><strong>localhost</strong><small>站点访问</small></span><b className="allowed">已允许</b></div>
    <div className="inspector-heading permission-subheading">电脑操作</div>
    <div className="permission-row"><Globe2 size={17}/><span><strong>Google Chrome</strong><small>应用程序</small></span><b className={decision === 'allow' ? 'allowed' : decision === 'deny' ? 'denied' : 'pending'}>{decision === 'allow' ? '已允许' : decision === 'deny' ? '已拒绝' : '待批准'}</b></div>
    <div className="permission-row"><TextCursorInput size={17}/><span><strong>安全文本输入</strong><small>密码框始终禁止</small></span><b className="allowed">强制策略</b></div>
    <div className="permission-row"><Settings size={17}/><span><strong>系统设置</strong><small>应用程序</small></span><b className="denied">已拒绝</b></div>
    {decision === 'ask' ? <div className="computer-approval"><MonitorCog size={21}/><strong>允许 Aether 控制当前前台 Chrome 吗？</strong><p>一次性授权将在下一次Computer Use调用后自动失效；macOS Accessibility仍需单独授权。</p><button className="primary-button" onClick={() => void decide(true, 'once')}>仅允许本次</button><button className="secondary-button" onClick={() => void decide(true, 'persistent')}>始终允许 Chrome</button><button className="secondary-button" onClick={() => void decide(false, 'persistent')}>拒绝</button></div> : null}
    <div className="permission-row"><CheckCircle2 size={17}/><span><strong>屏幕录制</strong><small>macOS 权限</small></span><b className={native.screen_recording ? 'allowed' : 'pending'}>{native.screen_recording ? '已授予' : '未授予'}</b></div>
    <div className="permission-row"><CircleAlert size={17}/><span><strong>辅助功能</strong><small>macOS 权限</small></span><b className={native.accessibility ? 'allowed' : 'pending'}>{native.accessibility ? '已授予' : '未授予'}</b></div>
    {!native.accessibility || !native.screen_recording ? <button className="link-button" onClick={() => void refresh(true)}>请求macOS权限</button> : null}
    {decision === 'allow' ? <section className="computer-controls">
      <div className="agent-actions"><button onClick={() => void operate('tree')}>读取AX树</button><button onClick={() => void operate('screenshot')}>截取前台应用</button></div>
      <label>X<input inputMode="decimal" value={point.x} onChange={(event) => setPoint((value) => ({ ...value, x: event.target.value }))}/></label><label>Y<input inputMode="decimal" value={point.y} onChange={(event) => setPoint((value) => ({ ...value, y: event.target.value }))}/></label>
      <label>输入文本<input value={text} onChange={(event) => setText(event.target.value)}/></label>
      <label className="sensitive-confirm"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)}/>确认执行点击或输入</label>
      <div className="agent-actions"><button disabled={!confirmed || !Number.isFinite(Number(point.x)) || !Number.isFinite(Number(point.y))} onClick={() => void operate('click')}>点击坐标</button><button disabled={!confirmed || !text} onClick={() => void operate('type')}>输入文本</button></div>
    </section> : null}
    {tree ? <pre className="computer-tree">{tree}</pre> : null}{screenshot ? <img className="browser-screenshot" src={screenshot} alt="Computer Use前台应用截图"/> : null}
    {error ? <div className="backend-error">{error}</div> : null}
    <button className="revoke-button" onClick={() => void decide(false, 'persistent')}>撤销Chrome控制权限</button>
  </div>
}

function AgentInspector() {
  return <div className="inspector-content"><div className="inspector-heading">活动 Agent <span>0</span></div><p className="empty-agent-state">尚无真实Agent。选择项目并启动任务后，此处显示独立线程、工作区和运行状态。</p></div>
}

function EvidenceInspector({ compact = false }: { compact?: boolean }) {
  const project = useWorkspace((state) => state.project)
  const gates = useWorkspace((state) => state.releaseGates)
  if (project) {
    const profiles = ['foundation', 'desktop', 'agent', 'simulation', 'simulation-extensions', 'software', 'development-package', 'production'] as const
    return <div className={compact ? 'evidence-block compact' : 'inspector-content evidence-content'}>
      {!compact ? <div className="inspector-heading">发布门</div> : <h3>发布门</h3>}
      {profiles.map((profile) => { const gate = gates[profile]; const passed = gate?.passed === true; return <div className={`evidence-row ${passed ? 'good' : 'bad'}`} key={profile}>{passed ? <CheckCircle2 size={20}/> : <CircleX size={20}/>}<span><strong>{profile}</strong><small>{gate ? passed ? '已通过' : `${gate.failures.length} 个缺口` : '检查中…'}</small></span></div> })}
      {!compact && gates.production && !gates.production.passed ? <><p className="fidelity-warning">Production 未通过时不得发布 1.0，也不得声明真机等价。</p><details className="gate-evidence"><summary>查看发布门证据</summary><pre>{JSON.stringify(gates, null, 2)}</pre></details></> : null}
    </div>
  }
  return <div className={compact ? 'evidence-block compact' : 'inspector-content evidence-content'}>
    {!compact ? <div className="inspector-heading">证据</div> : <h3>证据</h3>}
    <div className="evidence-row warn"><CircleAlert size={20} /><span><strong>尚无运行证据</strong><small>选择项目并执行实验后生成Evidence Bundle</small></span></div>
    {!compact ? <div className="evidence-row bad"><CircleX size={20} /><span><strong>硬件与生产声明不可用</strong><small>需要签名Validation Envelope</small></span></div> : null}
  </div>
}

function ContextInspector() {
  const project = useWorkspace((state) => state.project)
  const selectedThread = useWorkspace((state) => state.selectedThread)
  if (!project) return <div className="inspector-content"><div className="inspector-heading">上下文预览</div><div className="context-stat"><Activity size={17} /><span><strong>未选择项目</strong><small>不会显示伪造Token统计</small></span></div><dl className="fidelity-list"><dt>项目规则</dt><dd>等待项目</dd><dt>权限</dt><dd>未授予</dd><dt>模型</dt><dd>gpt-5.6</dd></dl></div>
  return <div className="inspector-content"><div className="inspector-heading">真实上下文</div><div className="context-stat"><Activity size={17}/><span><strong>{project.root.split('/').at(-1)}</strong><small>AGENTS规则按目录动态加载</small></span></div><dl className="fidelity-list"><dt>项目ID</dt><dd>{project.id.slice(0, 12)}</dd><dt>权限</dt><dd>{project.permission}</dd><dt>当前任务</dt><dd>{selectedThread ? selectedThread.slice(0, 8) : '无'}</dd></dl><MemoryPanel projectID={project.id} threadID={selectedThread}/></div>
}

function MemoryPanel({ projectID, threadID }: { projectID: string; threadID: string }) {
  const [enabled, setEnabled] = useState(false)
  const [memories, setMemories] = useState<AetherMemory[]>([])
  const [content, setContent] = useState('')
  const [error, setError] = useState('')
  const refresh = async () => {
    const api = backend()
    if (!api) return
    try {
      const status = await api.MemoryStatus()
      setEnabled(status.project)
      setMemories(status.project ? await api.ListMemories('project') : [])
      setError('')
    } catch (reason) {
      setError(String(reason))
    }
  }
  useEffect(() => { void refresh() }, [projectID])
  const toggle = async () => {
    const api = backend()
    if (!api) return
    await api.SetMemoryEnabled('project', !enabled)
    await refresh()
  }
  const save = async () => {
    const api = backend()
    if (!api || !content.trim()) return
    try {
      await api.SaveMemory('project', content.trim(), threadID)
      setContent('')
      await refresh()
    } catch (reason) {
      setError(String(reason))
    }
  }
  return <section className="memory-panel"><header><strong>本地项目记忆</strong><button className={enabled ? 'allowed' : ''} onClick={() => void toggle()}>{enabled ? '已启用' : '选择加入'}</button></header>{enabled ? <><textarea value={content} onChange={(event) => setContent(event.target.value)} placeholder="写入会先脱敏，且不能覆盖AGENTS规则…"/><button className="primary-button" disabled={!content.trim()} onClick={() => void save()}>保存记忆</button>{memories.map((memory) => <article key={memory.id}><p>{memory.content}</p><button onClick={async () => { await backend()?.DeleteMemory(memory.id); await refresh() }}>删除</button></article>)}</> : <p>默认关闭。启用后每轮最多加载最近20条脱敏记忆。</p>}{error ? <div className="backend-error">{error}</div> : null}</section>
}
import { useEffect, useState } from 'react'
