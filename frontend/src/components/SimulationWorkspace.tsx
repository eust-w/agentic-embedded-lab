import { Activity, CheckCircle2, CircleX, Network, RadioTower, Thermometer, Zap } from 'lucide-react'

const timeline = ['Reset deasserted', 'Boot ROM start', 'Firmware init', 'High-load traffic start', 'UART RX burst', 'UART RX overrun injected', 'ISR entry', 'Error flag set', 'Stats update', 'End of simulation']

export function SimulationWorkspace() {
  return <section className="simulation-workspace">
    <header className="simulation-header"><span className="sim-icon"><Activity size={18} /></span><div><strong>UART overrun investigation</strong><small>Virtual time <b>00:18.742912</b> · deterministic</small></div><dl><dt>Seed</dt><dd>0xA7E1_5EED</dd><dt>Scenario</dt><dd>uart_overrun_high_load.repl</dd></dl><button className="secondary-button">Configure…</button></header>
    <div className="simulation-grid">
      <section className="timeline-panel"><h3>Experiment timeline</h3>{timeline.map((event, index) => <div className={`timeline-event ${index === 5 ? 'error' : ''}`} key={event}><span /><time>00:{String(index * 2).padStart(2, '0')}.{String(index * 3176).padStart(6, '0').slice(0, 6)}</time><p>{event}</p></div>)}</section>
      <section className="topology-panel"><h3>Topology · co-simulation</h3><div className="topology-node primary">Firmware / Renode<small>Cortex-M4F, HAL, Drivers</small></div><div className="topology-row"><div className="topology-node"><Zap size={15} /> ngspice<small>Power rails</small></div><div className="topology-node"><Thermometer size={15} /> Modelica<small>Thermal</small></div></div><div className="topology-node"><Network size={15} /> ns-3<small>Network traffic</small></div><div className="topology-node"><RadioTower size={15} /> openEMS<small>EM / SI fields</small></div></section>
      <section className="plots-panel"><header><button className="active">Plots</button><button>Signals</button><button>Logs</button><span>Synchronized · 1 ms/div</span></header><div className="plot-grid"><Plot title="Rail voltage — VDD (V)" tone="blue" /><Plot title="Temperature — MCU (°C)" tone="orange" /><Plot title="Packet retries — ns-3" tone="violet" /><Plot title="UART events" tone="cyan" /></div></section>
    </div>
    <div className="simulation-bottom"><section><h3>Assertions</h3>{[['No UART ORE under nominal load','fail'],['VDD never below 3.0 V','pass'],['MCU temp below 85 °C','pass'],['Packet retry rate < 5%','fail'],['No deadlocks','pass']].map(([label,status], index) => <div className={`assertion-row ${status}`} key={label}><code>A0{index+1}</code><span>{label}</span>{status === 'pass' ? <CheckCircle2 size={15}/> : <CircleX size={15}/>}<b>{status}</b></div>)}</section><section><h3>Evidence artifacts</h3>{['trace.uart.bin','rails.vcd.gz','temps.csv','ns3-pcap.pcapng','openems.h5'].map((name,index) => <div className="artifact-row" key={name}><span>{name}</span><small>{['trace','waveform','timeseries','pcap','field'][index]}</small><code>{`${index+1}e2f3a4b5c6d7e8091…`}</code></div>)}</section></div>
  </section>
}

function Plot({ title, tone }: { title: string; tone: string }) {
  return <div className="plot"><h4>{title}</h4><svg viewBox="0 0 240 100" preserveAspectRatio="none" aria-label={title}><g className="grid-lines"><path d="M0 20H240M0 50H240M0 80H240M40 0V100M100 0V100M160 0V100M220 0V100" /></g><polyline className={tone} points="0,65 14,58 26,62 39,42 52,54 66,49 82,60 96,35 111,46 124,30 138,51 153,43 168,38 184,52 199,34 218,42 240,25" /></svg></div>
}
