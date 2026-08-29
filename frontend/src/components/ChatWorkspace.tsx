import { Check, ChevronRight, Circle, FileCode2, ShieldCheck } from 'lucide-react'
import { useWorkspace } from '../store/workspace'
import type { WorkspaceView } from '../types'

const plan = [
  ['在仿真中复现溢出', 'done'],
  ['检查 UART ISR 与环形缓冲区', 'active'],
  ['调整 RX 路径与缓冲策略', 'pending'],
  ['重新仿真并记录证据', 'pending'],
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
    <section className="chat-workspace" aria-label={mode === 'diff' ? '变更工作区' : '对话工作区'}>
      <div className="assistant-heading">
        <div className="aether-mark">A</div>
        <div><strong>Aether</strong><span>2 分钟前</span><p>正在修复嵌入式固件时序问题</p><small>高 ISR 负载下，UART RX 在 115200 波特率发生溢出。</small></div>
      </div>
      <div className="plan-panel">
        <div className="panel-title">执行计划</div>
        {plan.map(([label, status], index) => (
          <div className={`plan-row ${status}`} key={label}>
            <span className="plan-index">{status === 'done' ? <Check size={13} /> : index + 1}</span>
            <span>{label}</span><small>{status === 'done' ? '仿真已验证' : status === 'active' ? '进行中' : '等待中'}</small>
          </div>
        ))}
      </div>
      <div className="tool-calls">
        {calls.map(([name, target, duration]) => (
          <div className="tool-call" key={`${name}-${target}`}><ChevronRight size={14} /><span>工具调用</span><code>{name}</code><small>{target}</small><Check size={13} className="success-icon" /><time>{duration}</time></div>
        ))}
      </div>
      {approval.status === 'pending' ? (
        <div className="approval-card">
          <ShieldCheck size={21} />
          <div className="approval-copy"><strong>{approval.title}</strong><p>{approval.description}</p><div className="resource-list">{approval.resources.map((item) => <code key={item}>{item}</code>)}</div><small><b>+{approval.additions}</b> <em>−{approval.deletions}</em></small></div>
          <button className="primary-button" onClick={() => decide('approved')}>仅批准本次</button>
          <button className="secondary-button" onClick={() => decide('denied')}>拒绝</button>
        </div>
      ) : (
        <div className={`decision-banner ${approval.status}`}><ShieldCheck size={18} />{approval.status === 'approved' ? '已批准当前轮次' : '已拒绝变更'}</div>
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
      <div className="composer-placeholder"><Circle size={12} /> Agent 正在等待批准，批准后才会应用变更。</div>
    </section>
  )
}
