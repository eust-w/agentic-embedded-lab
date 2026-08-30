import { Circle, FileCode2, LoaderCircle, Paperclip, Send, ShieldCheck, X } from 'lucide-react'
import { useWorkspace } from '../store/workspace'
import type { WorkspaceView } from '../types'

export function ChatWorkspace({ mode }: { mode: WorkspaceView }) {
  const project = useWorkspace((state) => state.project)
  const items = useWorkspace((state) => state.items)
  const input = useWorkspace((state) => state.input)
  const busy = useWorkspace((state) => state.busy)
  const backendError = useWorkspace((state) => state.backendError)
  const setInput = useWorkspace((state) => state.setInput)
  const submit = useWorkspace((state) => state.submit)
  const attachments = useWorkspace((state) => state.attachments)
  const pickAttachments = useWorkspace((state) => state.pickAttachments)
  const removeAttachment = useWorkspace((state) => state.removeAttachment)
  const resolveLiveApproval = useWorkspace((state) => state.resolveLiveApproval)
  const liveApproval = [...items].reverse().find((item) => item.type === 'approval')
  const liveRequest = liveApproval?.payload.request as { id?: string; tool?: string; reason?: string; resource?: string } | undefined
  return (
    <section className="chat-workspace" aria-label={mode === 'diff' ? '变更工作区' : '对话工作区'}>
      <div className="assistant-heading">
        <div className="aether-mark">A</div>
        <div><strong>Aether</strong><span>{project ? '实时任务' : '未连接项目'}</span><p>{project ? `项目：${project.root.split('/').at(-1)}` : '选择项目后开始真实工程任务'}</p><small>{project ? '模型输出、工具结果与审批记录均来自本地 aetherd。' : '此处不会展示演示任务、模拟工具调用或伪造证据。'}</small></div>
      </div>
      {project ? <div className="live-feed" aria-live="polite">
        {items.length === 0 ? <div className="empty-live-state">输入任务后，真实模型输出、工具调用和审批请求会显示在这里。</div> : items.map((item) => <article className={`live-item ${item.type}`} key={item.id}>
          <small>{item.type === 'user_message' ? '你' : item.type === 'agent_message' ? 'Aether' : item.type === 'tool_result' ? '工具结果' : item.type === 'approval' ? '审批请求' : '执行事件'}</small>
          <p>{renderItem(item.payload)}</p>
        </article>)}
      </div> : null}
      {!project ? <div className="empty-live-state">尚无真实任务。请先启动后台并选择一个本地项目工作区。</div> : null}
      {project && liveRequest?.id ? <div className="approval-card live-approval">
        <ShieldCheck size={21} />
        <div className="approval-copy"><strong>工具请求批准：{liveRequest.tool}</strong><p>{liveRequest.reason}</p><code>{liveRequest.resource}</code></div>
        <button className="primary-button" onClick={() => void resolveLiveApproval(liveRequest.id!, true)}>仅批准本次</button>
        <button className="secondary-button" onClick={() => void resolveLiveApproval(liveRequest.id!, false)}>拒绝</button>
      </div> : null}
      {backendError ? <div className="backend-error">{backendError}</div> : null}
      {attachments.length > 0 ? <div className="attachment-list">{attachments.map((attachment) => <span key={attachment.sha256}><FileCode2 size={12}/>{attachment.name}<small>{Math.ceil(attachment.bytes / 1024)} KiB</small><button aria-label={`移除附件${attachment.name}`} onClick={() => removeAttachment(attachment.sha256)}><X size={11}/></button></span>)}</div> : null}
      <form className={`live-composer ${project ? 'connected' : ''}`} onSubmit={(event) => { event.preventDefault(); void submit() }}>
        <Circle size={12} />
        <button className="attachment-button" type="button" aria-label="添加图片附件" disabled={!project || busy} onClick={() => void pickAttachments()}><Paperclip size={15}/></button>
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
