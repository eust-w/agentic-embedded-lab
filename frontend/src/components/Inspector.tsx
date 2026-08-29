import { Activity, CheckCircle2, CircleAlert, CircleX, GitBranch, Globe2, MonitorCog, Settings, ShieldCheck, TextCursorInput } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

export function Inspector() {
  const tab = useWorkspace((state) => state.inspectorTab)
  const view = useWorkspace((state) => state.view)
  const setTab = useWorkspace((state) => state.setInspectorTab)
  const agents = useWorkspace((state) => state.agents)
  return (
    <aside className="inspector">
      <nav className="inspector-tabs">
        {(['context', 'agents', 'evidence'] as const).map((item) => <button key={item} className={tab === item ? 'active' : ''} onClick={() => setTab(item)}>{item[0].toUpperCase() + item.slice(1)}</button>)}
      </nav>
      {view === 'browser' ? <PermissionInspector /> : tab === 'agents' ? <AgentInspector agents={agents} /> : tab === 'evidence' ? <EvidenceInspector /> : <ContextInspector />}
    </aside>
  )
}

function PermissionInspector() {
  return <div className="inspector-content permission-content">
    <div className="inspector-heading">Permissions</div>
    <div className="permission-row"><ShieldCheck size={17}/><span><strong>localhost</strong><small>Site access</small></span><b className="allowed">Allowed</b></div>
    <div className="inspector-heading permission-subheading">Computer Use</div>
    <div className="permission-row"><Globe2 size={17}/><span><strong>Google Chrome</strong><small>Current tab</small></span><b className="pending">Approval</b></div>
    <div className="permission-row"><TextCursorInput size={17}/><span><strong>TextEdit</strong><small>Application</small></span><b className="allowed">Allowed</b></div>
    <div className="permission-row"><Settings size={17}/><span><strong>System Settings</strong><small>Application</small></span><b className="denied">Denied</b></div>
    <div className="computer-approval"><MonitorCog size={21}/><strong>Allow Aether to control Chrome for this task?</strong><p>Aether wants to interact with the current Chrome tab to complete this step.</p><button className="primary-button">Allow once</button><button className="secondary-button">Always allow for Chrome</button><button className="secondary-button">Deny</button></div>
    <div className="permission-row"><CheckCircle2 size={17}/><span><strong>Screen Recording</strong><small>macOS permission</small></span><b className="allowed">Granted</b></div>
    <div className="permission-row"><CircleAlert size={17}/><span><strong>Accessibility</strong><small>macOS permission</small></span><b className="pending">Pending</b></div>
    <button className="revoke-button">Revoke all control</button>
  </div>
}

function AgentInspector({ agents }: { agents: ReturnType<typeof useWorkspace.getState>['agents'] }) {
  return <div className="inspector-content"><div className="inspector-heading">Active agents <span>{agents.length}</span></div>{agents.map((agent) => (
    <article className="agent-card" key={agent.id}>
      <header><span className={`agent-avatar ${agent.tone}`}>{agent.name[0]}</span><div><strong>{agent.name}</strong><small>{agent.status}</small></div><span className="status-dot online" /></header>
      <div className="progress-track"><i style={{ width: `${agent.progress}%` }} /></div>
      <p>{agent.detail}</p><small>Worktree</small><a>{agent.worktree}</a><GitBranch size={13} />
    </article>
  ))}<EvidenceInspector compact /></div>
}

function EvidenceInspector({ compact = false }: { compact?: boolean }) {
  return <div className={compact ? 'evidence-block compact' : 'inspector-content evidence-content'}>
    {!compact ? <div className="inspector-heading">Evidence</div> : <h3>Evidence</h3>}
    <div className="evidence-row good"><CheckCircle2 size={20} /><span><strong>Simulation validated</strong><small>Renode 1.16.1 · just now</small></span></div>
    <div className="evidence-row warn"><CircleAlert size={20} /><span><strong>Hardware unverified</strong><small>No hardware runs yet</small></span></div>
    {!compact ? <div className="evidence-row bad"><CircleX size={20} /><span><strong>Production claim blocked</strong><small>Hardware evidence is required</small></span></div> : null}
    {!compact ? <><dl className="fidelity-list"><dt>Firmware</dt><dd>functional</dd><dt>Timing</dt><dd>model-dependent</dd><dt>Physical</dt><dd>unverified</dd></dl><button className="link-button">Open evidence log</button></> : null}
  </div>
}

function ContextInspector() {
  return <div className="inspector-content"><div className="inspector-heading">Context</div><div className="context-stat"><Activity size={17} /><span><strong>14 files</strong><small>38.2k weighted tokens</small></span></div><dl className="fidelity-list"><dt>Instructions</dt><dd>AGENTS.md</dd><dt>Workspace</dt><dd>feature/uart-fix</dd><dt>Permission</dt><dd>workspace write</dd><dt>Model</dt><dd>gpt-5.6</dd></dl></div>
}
