import { Activity, CheckCircle2, CircleX, Network, RadioTower, Thermometer, Zap } from 'lucide-react'
import { useWorkspace } from '../store/workspace'

const timeline = ['复位解除', 'Boot ROM 启动', '固件初始化', '高负载流量开始', 'UART RX 突发', '注入 UART RX 溢出', '进入 ISR', '设置错误标志', '更新统计信息', '仿真结束']

export function SimulationWorkspace() {
  const run = useWorkspace((state) => state.experimentRun)
  const backendError = useWorkspace((state) => state.backendError)
  const liveTimeline = run?.bundle?.events.slice(-10).map((event) => ({ label: `${event.source} · ${event.type}`, time: event.virtual_time_us, error: event.type.includes('fail') || event.type.includes('error') }))
  const assertions = run?.bundle?.assertions ?? [['标称负载下不得出现 UART ORE','fail'],['VDD 始终不低于 3.0 V','pass'],['MCU 温度低于 85 °C','pass'],['数据包重试率低于 5%','fail'],['无死锁','pass']]
  const artifacts = run?.bundle ? Object.entries(run.bundle.artifacts) : ['trace.uart.bin','rails.vcd.gz','temps.csv','ns3-pcap.pcapng','openems.h5'].map((name, index) => [name, `${index+1}e2f3a4b5c6d7e8091…`])
  return <section className="simulation-workspace">
    <header className="simulation-header"><span className="sim-icon"><Activity size={18} /></span><div><strong>{run ? 'AEL 五域联合实验' : 'UART 溢出问题调查（离线预览）'}</strong><small>{run ? <>状态 <b>{run.status}</b> · {run.bundle?.trace_sha256 ? `Trace ${run.bundle.trace_sha256.slice(0, 12)}…` : '等待证据'}</> : <>虚拟时间 <b>00:18.742912</b> · 确定性运行</>}</small></div><dl><dt>运行 ID</dt><dd>{run?.id.slice(0, 12) ?? 'preview'}</dd><dt>证据边界</dt><dd>{run?.bundle?.fidelity.hardware_validated ? '真机已验证' : '硬件未验证'}</dd></dl><button className="secondary-button">配置…</button></header>
    <div className="simulation-grid">
      <section className="timeline-panel"><h3>实验时间线</h3>{liveTimeline?.length ? liveTimeline.map((event) => <div className={`timeline-event ${event.error ? 'error' : ''}`} key={`${event.time}-${event.label}`}><span /><time>{(event.time / 1_000_000).toFixed(6)}s</time><p>{event.label}</p></div>) : timeline.map((event, index) => <div className={`timeline-event ${index === 5 ? 'error' : ''}`} key={event}><span /><time>00:{String(index * 2).padStart(2, '0')}.{String(index * 3176).padStart(6, '0').slice(0, 6)}</time><p>{event}</p></div>)}</section>
      <section className="topology-panel"><h3>拓扑 · 联合仿真</h3><div className="topology-node primary">固件 / Renode<small>Cortex-M4F、HAL、驱动</small></div><div className="topology-row"><div className="topology-node"><Zap size={15} /> ngspice<small>电源轨</small></div><div className="topology-node"><Thermometer size={15} /> Modelica<small>热模型</small></div></div><div className="topology-node"><Network size={15} /> ns-3<small>网络流量</small></div><div className="topology-node"><RadioTower size={15} /> openEMS<small>电磁 / 信号完整性场</small></div></section>
      <section className="plots-panel"><header><button className="active">曲线</button><button>信号</button><button>日志</button><span>已同步 · 1 ms/格</span></header><div className="plot-grid"><Plot title="电源轨电压 — VDD (V)" tone="blue" /><Plot title="MCU 温度 (°C)" tone="orange" /><Plot title="数据包重试 — ns-3" tone="violet" /><Plot title="UART 事件" tone="cyan" /></div></section>
    </div>
    <div className="simulation-bottom"><section><h3>断言</h3>{assertions.map((assertion, index) => { const live = !Array.isArray(assertion); const label = live ? assertion.message : assertion[0]; const passed = live ? assertion.passed : assertion[1] === 'pass'; const id = live ? assertion.id : `A0${index+1}`; return <div className={`assertion-row ${passed ? 'pass' : 'fail'}`} key={id}><code>{id}</code><span>{label}</span>{passed ? <CheckCircle2 size={15}/> : <CircleX size={15}/>}<b>{passed ? '通过' : '失败'}</b></div> })}</section><section><h3>证据产物</h3>{artifacts.map(([name, hash],index) => <div className="artifact-row" key={name}><span>{name}</span><small>{run ? '可追溯产物' : ['追踪','波形','时间序列','网络抓包','场数据'][index]}</small><code>{hash}</code></div>)}</section></div>
    {backendError ? <div className="backend-error simulation-error">{backendError}</div> : null}
  </section>
}

function Plot({ title, tone }: { title: string; tone: string }) {
  return <div className="plot"><h4>{title}</h4><svg viewBox="0 0 240 100" preserveAspectRatio="none" aria-label={title}><g className="grid-lines"><path d="M0 20H240M0 50H240M0 80H240M40 0V100M100 0V100M160 0V100M220 0V100" /></g><polyline className={tone} points="0,65 14,58 26,62 39,42 52,54 66,49 82,60 96,35 111,46 124,30 138,51 153,43 168,38 184,52 199,34 218,42 240,25" /></svg></div>
}
