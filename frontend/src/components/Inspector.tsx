import { Activity, CheckCircle2, CircleAlert, CircleX, GitBranch, Globe2, MonitorCog, Settings, ShieldCheck, TextCursorInput } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

const tabLabels = { context: '上下文', agents: 'Agent', evidence: '证据' }
const statusLabels = { working: '工作中', running: '运行中', idle: '空闲' }

export function Inspector() {
  const tab = useWorkspace((state) => state.inspectorTab)
  const view = useWorkspace((state) => state.view)
  const setTab = useWorkspace((state) => state.setInspectorTab)
  const agents = useWorkspace((state) => state.agents)
  return (
    <aside className="inspector">
      <nav className="inspector-tabs">
        {(['context', 'agents', 'evidence'] as const).map((item) => <button key={item} className={tab === item ? 'active' : ''} onClick={() => setTab(item)}>{tabLabels[item]}</button>)}
      </nav>
      {view === 'browser' ? <PermissionInspector /> : tab === 'agents' ? <AgentInspector agents={agents} /> : tab === 'evidence' ? <EvidenceInspector /> : <ContextInspector />}
    </aside>
  )
}

function PermissionInspector() {
  return <div className="inspector-content permission-content">
    <div className="inspector-heading">权限</div>
    <div className="permission-row"><ShieldCheck size={17}/><span><strong>localhost</strong><small>站点访问</small></span><b className="allowed">已允许</b></div>
    <div className="inspector-heading permission-subheading">电脑操作</div>
    <div className="permission-row"><Globe2 size={17}/><span><strong>Google Chrome</strong><small>当前标签页</small></span><b className="pending">待批准</b></div>
    <div className="permission-row"><TextCursorInput size={17}/><span><strong>文本编辑</strong><small>应用程序</small></span><b className="allowed">已允许</b></div>
    <div className="permission-row"><Settings size={17}/><span><strong>系统设置</strong><small>应用程序</small></span><b className="denied">已拒绝</b></div>
    <div className="computer-approval"><MonitorCog size={21}/><strong>允许 Aether 在本任务中控制 Chrome 吗？</strong><p>Aether 需要与当前 Chrome 标签页交互，才能完成此步骤。</p><button className="primary-button">仅允许本次</button><button className="secondary-button">始终允许 Chrome</button><button className="secondary-button">拒绝</button></div>
    <div className="permission-row"><CheckCircle2 size={17}/><span><strong>屏幕录制</strong><small>macOS 权限</small></span><b className="allowed">已授予</b></div>
    <div className="permission-row"><CircleAlert size={17}/><span><strong>辅助功能</strong><small>macOS 权限</small></span><b className="pending">等待中</b></div>
    <button className="revoke-button">撤销全部控制权限</button>
  </div>
}

function AgentInspector({ agents }: { agents: ReturnType<typeof useWorkspace.getState>['agents'] }) {
  return <div className="inspector-content"><div className="inspector-heading">活动 Agent <span>{agents.length}</span></div>{agents.map((agent) => (
    <article className="agent-card" key={agent.id}>
      <header><span className={`agent-avatar ${agent.tone}`}>{agent.name[0]}</span><div><strong>{agent.name}</strong><small>{statusLabels[agent.status]}</small></div><span className="status-dot online" /></header>
      <div className="progress-track"><i style={{ width: `${agent.progress}%` }} /></div>
      <p>{agent.detail}</p><small>工作树</small><a>{agent.worktree}</a><GitBranch size={13} />
    </article>
  ))}<EvidenceInspector compact /></div>
}

function EvidenceInspector({ compact = false }: { compact?: boolean }) {
  const project = useWorkspace((state) => state.project)
  const gates = useWorkspace((state) => state.releaseGates)
  if (project) {
    const profiles = ['foundation', 'simulation', 'software', 'production'] as const
    return <div className={compact ? 'evidence-block compact' : 'inspector-content evidence-content'}>
      {!compact ? <div className="inspector-heading">发布门</div> : <h3>发布门</h3>}
      {profiles.map((profile) => { const gate = gates[profile]; const passed = gate?.passed === true; return <div className={`evidence-row ${passed ? 'good' : 'bad'}`} key={profile}>{passed ? <CheckCircle2 size={20}/> : <CircleX size={20}/>}<span><strong>{profile}</strong><small>{gate ? passed ? '已通过' : `${gate.failures.length} 个缺口` : '检查中…'}</small></span></div> })}
      {!compact && gates.production && !gates.production.passed ? <><p className="fidelity-warning">Production 未通过时不得发布 1.0，也不得声明真机等价。</p><button className="link-button">查看发布门证据</button></> : null}
    </div>
  }
  return <div className={compact ? 'evidence-block compact' : 'inspector-content evidence-content'}>
    {!compact ? <div className="inspector-heading">证据</div> : <h3>证据</h3>}
    <div className="evidence-row good"><CheckCircle2 size={20} /><span><strong>仿真已验证</strong><small>Renode 1.16.1 · 刚刚</small></span></div>
    <div className="evidence-row warn"><CircleAlert size={20} /><span><strong>硬件尚未验证</strong><small>暂无真机运行记录</small></span></div>
    {!compact ? <div className="evidence-row bad"><CircleX size={20} /><span><strong>生产声明已阻止</strong><small>必须提供真机证据</small></span></div> : null}
    {!compact ? <><dl className="fidelity-list"><dt>固件</dt><dd>功能级</dd><dt>时序</dt><dd>依赖模型</dd><dt>物理</dt><dd>未验证</dd></dl><button className="link-button">打开证据日志</button></> : null}
  </div>
}

function ContextInspector() {
  return <div className="inspector-content"><div className="inspector-heading">上下文</div><div className="context-stat"><Activity size={17} /><span><strong>14 个文件</strong><small>38.2k 加权 Token</small></span></div><dl className="fidelity-list"><dt>项目规则</dt><dd>AGENTS.md</dd><dt>工作区</dt><dd>feature/uart-fix</dd><dt>权限</dt><dd>工作区写入</dd><dt>模型</dt><dd>gpt-5.6</dd></dl></div>
}
