import { Check, ChevronRight, Circle, FileCode2, ShieldCheck } from 'lucide-react'
import { useWorkspace } from '../store/workspace'
import type { WorkspaceView } from '../types'

const plan = [
  ['Reproduce overrun in simulation', 'done'],
  ['Inspect UART ISR and ring buffer', 'active'],
  ['Adjust RX path and buffering', 'pending'],
  ['Validate in simulation and record evidence', 'pending'],
]

const calls = [
  ['run_simulation', 'Renode', '1.2s'],
  ['read_file', 'src/uart.c', '320ms'],
  ['search', 'HAL_UART_RxCpltCallback', '210ms'],
  ['read_file', 'src/uart_isr.c', '180ms'],
]

export function ChatWorkspace({ mode }: { mode: WorkspaceView }) {
  const approval = useWorkspace((state) => state.approval)
  const decide = useWorkspace((state) => state.decideApproval)
  return (
    <section className="chat-workspace" aria-label={mode === 'diff' ? 'Diff workspace' : 'Chat workspace'}>
      <div className="assistant-heading">
        <div className="aether-mark">A</div>
        <div><strong>Aether</strong><span>2m ago</span><p>Fixing embedded firmware timing issue</p><small>UART RX overrun at 115200 baud under high ISR load.</small></div>
      </div>
      <div className="plan-panel">
        <div className="panel-title">Plan</div>
        {plan.map(([label, status], index) => (
          <div className={`plan-row ${status}`} key={label}>
            <span className="plan-index">{status === 'done' ? <Check size={13} /> : index + 1}</span>
            <span>{label}</span><small>{status === 'done' ? 'Simulation validated' : status === 'active' ? 'In progress' : 'Pending'}</small>
          </div>
        ))}
      </div>
      <div className="tool-calls">
        {calls.map(([name, target, duration]) => (
          <div className="tool-call" key={`${name}-${target}`}><ChevronRight size={14} /><span>Tool call</span><code>{name}</code><small>{target}</small><Check size={13} className="success-icon" /><time>{duration}</time></div>
        ))}
      </div>
      {approval.status === 'pending' ? (
        <div className="approval-card">
          <ShieldCheck size={21} />
          <div className="approval-copy"><strong>{approval.title}</strong><p>{approval.description}</p><div className="resource-list">{approval.resources.map((item) => <code key={item}>{item}</code>)}</div><small><b>+{approval.additions}</b> <em>−{approval.deletions}</em></small></div>
          <button className="primary-button" onClick={() => decide('approved')}>Approve once</button>
          <button className="secondary-button" onClick={() => decide('denied')}>Deny</button>
        </div>
      ) : (
        <div className={`decision-banner ${approval.status}`}><ShieldCheck size={18} />{approval.status === 'approved' ? 'Approved for this turn' : 'Change denied'}</div>
      )}
      <div className="diff-panel">
        <header><FileCode2 size={14} /> src/uart_isr.c <span>•••</span></header>
        <div className="diff-grid">
          <pre><i>128</i> if (__HAL_UART_GET_FLAG(&amp;huartx, UART_FLAG_RXNE)) {'{'}
<i>129</i>   uint8_t b = huartx.Instance→RDR;
<i>130</i>   rb_push(&amp;uart_rx_rb, b);
<mark className="removed"><i>131</i>   stats.rx_overrun++;</mark>
<i>132</i> {'}'}</pre>
          <pre><i>128</i> if (__HAL_UART_GET_FLAG(&amp;huartx, UART_FLAG_RXNE)) {'{'}
<i>129</i>   uint8_t b = huartx.Instance→RDR;
<mark className="added"><i>130</i>   if (!rb_push(&amp;uart_rx_rb, b)) {'{'}</mark>
<mark className="added"><i>131</i>     stats.rx_overrun++;</mark>
<mark className="added"><i>132</i>   {'}'}</mark></pre>
        </div>
      </div>
      <div className="composer-placeholder"><Circle size={12} /> Agent is waiting for approval before applying changes.</div>
    </section>
  )
}
