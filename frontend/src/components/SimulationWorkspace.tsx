import { Activity, CheckCircle2, CircleX, Network, RadioTower, Thermometer, Zap } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

export function SimulationWorkspace() {
  const run = useWorkspace((state) => state.experimentRun)
  const backendError = useWorkspace((state) => state.backendError)
  if (!run) return <section className="simulation-workspace simulation-empty"><header className="simulation-header"><span className="sim-icon"><Activity size={18}/></span><div><strong>尚无真实仿真运行</strong><small>选择项目并点击“运行实验”后，才会显示来自AEL Engine的时间线、断言和Evidence Bundle。</small></div><dl><dt>运行 ID</dt><dd>无</dd><dt>证据边界</dt><dd>未生成证据</dd></dl></header><div className="empty-live-state">不会展示演示波形、预设断言或伪造Trace。仿真通过也不等于真机通过。</div>{backendError ? <div className="backend-error simulation-error">{backendError}</div> : null}</section>
  const liveTimeline = run?.bundle?.events.slice(-10).map((event) => ({ label: `${event.source} · ${event.type}`, time: event.virtual_time_us, error: event.type.includes('fail') || event.type.includes('error') }))
  const assertions = run.bundle?.assertions ?? []
  const artifacts = run.bundle ? Object.entries(run.bundle.artifacts) : []
  return <section className="simulation-workspace">
    <header className="simulation-header"><span className="sim-icon"><Activity size={18} /></span><div><strong>AEL 五域联合实验</strong><small>状态 <b>{run.status}</b> · {run.bundle?.trace_sha256 ? `Trace ${run.bundle.trace_sha256.slice(0, 12)}…` : '等待证据'}</small></div><dl><dt>运行 ID</dt><dd>{run.id.slice(0, 12)}</dd><dt>证据边界</dt><dd>{run.bundle?.fidelity.hardware_validated ? '真机已验证' : '硬件未验证'}</dd></dl></header>
    <div className="simulation-grid">
      <section className="timeline-panel"><h3>实验时间线</h3>{liveTimeline?.length ? liveTimeline.map((event) => <div className={`timeline-event ${event.error ? 'error' : ''}`} key={`${event.time}-${event.label}`}><span /><time>{(event.time / 1_000_000).toFixed(6)}s</time><p>{event.label}</p></div>) : <p className="sidebar-empty">等待真实事件。</p>}</section>
      <section className="topology-panel"><h3>拓扑 · 联合仿真</h3><div className="topology-node primary">固件 / Renode<small>Cortex-M4F、HAL、驱动</small></div><div className="topology-row"><div className="topology-node"><Zap size={15} /> ngspice<small>电源轨</small></div><div className="topology-node"><Thermometer size={15} /> Modelica<small>热模型</small></div></div><div className="topology-node"><Network size={15} /> ns-3<small>网络流量</small></div><div className="topology-node"><RadioTower size={15} /> openEMS<small>电磁 / 信号完整性场</small></div></section>
      <section className="plots-panel"><header><button className="active">原始结果</button><span>仅显示Evidence Bundle中存在的产物</span></header><div className="empty-live-state">当前Bundle未提供可直接绘制的采样序列；请从证据产物读取原始波形或时间序列。</div></section>
    </div>
    <div className="simulation-bottom"><section><h3>断言</h3>{assertions.length ? assertions.map((assertion) => <div className={`assertion-row ${assertion.passed ? 'pass' : 'fail'}`} key={assertion.id}><code>{assertion.id}</code><span>{assertion.message}</span>{assertion.passed ? <CheckCircle2 size={15}/> : <CircleX size={15}/>}<b>{assertion.passed ? '通过' : '失败'}</b></div>) : <p className="sidebar-empty">尚无断言结果。</p>}</section><section><h3>证据产物</h3>{artifacts.length ? artifacts.map(([name, hash]) => <div className="artifact-row" key={name}><span>{name}</span><small>可追溯产物</small><code>{hash}</code></div>) : <p className="sidebar-empty">尚无证据产物。</p>}</section></div>
    {backendError ? <div className="backend-error simulation-error">{backendError}</div> : null}
  </section>
}
