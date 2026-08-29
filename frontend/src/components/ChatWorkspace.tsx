import { Check, ChevronRight, Circle, FileCode2, LoaderCircle, Send, ShieldCheck } from 'lucide-react'
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
  const project = useWorkspace((state) => state.project)
  const items = useWorkspace((state) => state.items)
  const input = useWorkspace((state) => state.input)
  const busy = useWorkspace((state) => state.busy)
  const backendError = useWorkspace((state) => state.backendError)
  const setInput = useWorkspace((state) => state.setInput)
  const submit = useWorkspace((state) => state.submit)
  const resolveLiveApproval = useWorkspace((state) => state.resolveLiveApproval)
  const liveApproval = [...items].reverse().find((item) => item.type === 'approval')
  const liveRequest = liveApproval?.payload.request as { id?: string; tool?: string; reason?: string; resource?: string } | undefined
  return (
    <section className="chat-workspace" aria-label={mode === 'diff' ? '变更工作区' : '对话工作区'}>
      <div className="assistant-heading">
        <div className="aether-mark">A</div>
        <div><strong>Aether</strong><span>{project ? '实时任务' : '离线预览'}</span><p>{project ? `项目：${project.root.split('/').at(-1)}` : '正在修复嵌入式固件时序问题'}</p><small>{project ? '模型输出、工具结果与审批记录均来自本地 aetherd。' : '高 ISR 负载下，UART RX 在 115200 波特率发生溢出。'}</small></div>
      </div>
      {project ? <div className="live-feed" aria-live="polite">
        {items.length === 0 ? <div className="empty-live-state">输入任务后，真实模型输出、工具调用和审批请求会显示在这里。</div> : items.map((item) => <article className={`live-item ${item.type}`} key={item.id}>
          <small>{item.type === 'user_message' ? '你' : item.type === 'agent_message' ? 'Aether' : item.type === 'tool_result' ? '工具结果' : item.type === 'approval' ? '审批请求' : '执行事件'}</small>
          <p>{renderItem(item.payload)}</p>
        </article>)}
      </div> : null}
      {!project ? <>
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
      </> : null}
      {project && liveRequest?.id ? <div className="approval-card live-approval">
        <ShieldCheck size={21} />
        <div className="approval-copy"><strong>工具请求批准：{liveRequest.tool}</strong><p>{liveRequest.reason}</p><code>{liveRequest.resource}</code></div>
        <button className="primary-button" onClick={() => void resolveLiveApproval(liveRequest.id!, true)}>仅批准本次</button>
        <button className="secondary-button" onClick={() => void resolveLiveApproval(liveRequest.id!, false)}>拒绝</button>
      </div> : null}
      {!project ? <>
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
      </> : null}
      {backendError ? <div className="backend-error">{backendError}</div> : null}
      <form className={`live-composer ${project ? 'connected' : ''}`} onSubmit={(event) => { event.preventDefault(); void submit() }}>
        <Circle size={12} />
        <textarea value={input} onChange={(event) => setInput(event.target.value)} placeholder={project ? '描述要完成的嵌入式开发任务…' : '先从左侧选择项目工作区'} disabled={!project || busy} rows={2} />
        <button className="primary-button" type="submit" disabled={!project || busy || !input.trim()}>{busy ? <LoaderCircle className="spin" size={16} /> : <Send size={16} />}{busy ? '执行中' : '发送'}</button>
      </form>
    </section>
  )
}

function renderItem(payload: Record<string, unknown>): string {
  if (typeof payload.text === 'string') return payload.text
  if (typeof payload.delta === 'string') return payload.delta
  if (typeof payload.error === 'string') return payload.error
  if (payload.tool && typeof payload.tool === 'string') return payload.tool
  return JSON.stringify(payload)
}
