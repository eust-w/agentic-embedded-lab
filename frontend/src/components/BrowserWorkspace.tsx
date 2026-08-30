import { useCallback, useEffect, useState } from 'react'
import { Camera, CircleAlert, CodeXml, Globe2, LockKeyhole, Play, RefreshCw, ShieldCheck, Square } from 'lucide-react'
import { backend } from '../lib/backend'
import type { BrowserConsoleEntry, BrowserNetworkEntry, BrowserStatus } from '../types'

type BrowserPanel = 'dom' | 'screenshot' | 'console' | 'network'

const initialStatus: BrowserStatus = { running: false, executable: '' }

export function BrowserWorkspace() {
  const [status, setStatus] = useState<BrowserStatus>(initialStatus)
  const [url, setURL] = useState('http://localhost:8080')
  const [panel, setPanel] = useState<BrowserPanel>('dom')
  const [dom, setDOM] = useState('')
  const [screenshot, setScreenshot] = useState('')
  const [consoleEntries, setConsoleEntries] = useState<BrowserConsoleEntry[]>([])
  const [networkEntries, setNetworkEntries] = useState<BrowserNetworkEntry[]>([])
  const [chromeSnapshot, setChromeSnapshot] = useState<{ url: string; title: string; dom: string; captured_at: string }>()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    const api = backend()
    if (!api) return
    const [nextStatus, nativeTab] = await Promise.all([
      api.BrowserStatus(),
      api.LatestChromeSnapshot().catch(() => ({ available: false, snapshot: undefined })),
    ])
    setStatus(nextStatus)
    setChromeSnapshot(nativeTab.available ? nativeTab.snapshot : undefined)
    if (!nextStatus.running || !nextStatus.url) return
    const [nextDOM, nextScreenshot, nextConsole, nextNetwork] = await Promise.all([
      api.BrowserDOM(),
      api.BrowserScreenshot(),
      api.BrowserConsole(0),
      api.BrowserNetwork(0),
    ])
    setDOM(nextDOM)
    setScreenshot(nextScreenshot)
    setConsoleEntries(nextConsole)
    setNetworkEntries(nextNetwork)
  }, [])

  useEffect(() => {
    void refresh().catch((reason: unknown) => setError(String(reason)))
  }, [refresh])

  const run = useCallback(async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
      await refresh()
    } catch (reason) {
      setError(String(reason))
    } finally {
      setBusy(false)
    }
  }, [refresh])

  const start = () => run(async () => {
    const api = backend()
    if (!api) throw new Error('Wails 后端不可用')
    await api.StartBrowser()
  })

  const stop = () => run(async () => {
    const api = backend()
    if (!api) throw new Error('Wails 后端不可用')
    await api.StopBrowser()
    setDOM('')
    setScreenshot('')
    setConsoleEntries([])
    setNetworkEntries([])
  })

  const navigate = () => run(async () => {
    const api = backend()
    if (!api) throw new Error('Wails 后端不可用')
    await api.NavigateBrowser(url)
  })

  const authorizeAndNavigate = () => run(async () => {
    const api = backend()
    if (!api) throw new Error('Wails 后端不可用')
    await api.SetSitePermission(url, true)
    await api.NavigateBrowser(url)
  })

  return <section className="browser-workspace real-browser-workspace">
    <header className="browser-bar">
      <Globe2 size={17}/>
      <label><LockKeyhole size={14}/><input aria-label="浏览器地址" value={url} onChange={(event) => setURL(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void navigate() }} /></label>
      <button disabled={busy || !status.running} onClick={() => void navigate()}><Play size={14}/> 打开</button>
      <button disabled={busy} onClick={() => void (status.running ? stop() : start())}>{status.running ? <Square size={13}/> : <Play size={13}/>} {status.running ? '停止浏览器' : '启动浏览器'}</button>
      <button disabled={busy || !status.running} onClick={() => void refresh()}><RefreshCw size={14}/> 刷新证据</button>
    </header>

    <nav className="browser-tools">
      <button className={panel === 'dom' ? 'active' : ''} onClick={() => setPanel('dom')}><CodeXml size={14}/> DOM</button>
      <button className={panel === 'screenshot' ? 'active' : ''} onClick={() => setPanel('screenshot')}><Camera size={14}/> 截图</button>
      <button className={panel === 'console' ? 'active' : ''} onClick={() => setPanel('console')}>控制台</button>
      <button className={panel === 'network' ? 'active' : ''} onClick={() => setPanel('network')}>网络</button>
      <span><i className={`status-dot ${status.running ? 'online' : ''}`}/> {status.running ? '受控 Chromium 已运行' : '浏览器未运行'}</span>
    </nav>

    {error ? <div className="browser-blocker">
      <CircleAlert size={17}/><span><strong>浏览器操作被阻止</strong>{error}</span>
      {error.includes('permission is ask') ? <button onClick={() => void authorizeAndNavigate()}><ShieldCheck size={14}/> 授权当前域名并打开</button> : null}
    </div> : null}

    {!status.running && chromeSnapshot ? <div className="browser-real-content">
      <header><strong>已授权的 Chrome 标签页：{chromeSnapshot.title || '未命名页面'}</strong><small>{chromeSnapshot.url}</small></header>
      <pre className="browser-dom-output">{chromeSnapshot.dom}</pre>
      <footer className="chrome-snapshot-boundary">该快照由用户点击 Chrome 扩展后采集；Aether 当前不能在未再次授权时持续控制该标签页。采集时间：{new Date(chromeSnapshot.captured_at).toLocaleString('zh-CN')}</footer>
    </div> : !status.running ? <div className="browser-empty-state">
      <Globe2 size={38}/>
      <h2>受控 Chromium 尚未启动</h2>
      <p>应用只使用打包内置的固定版本 Chromium。找不到可执行文件时会明确阻止，不会静默改用系统浏览器。</p>
      <code>{status.executable || '未解析 Chromium 路径'}</code>
      <button disabled={busy} onClick={() => void start()}>启动受控浏览器</button>
    </div> : <div className="browser-real-content">
      <header><strong>{status.title || '未命名页面'}</strong><small>{status.url || url}</small></header>
      {panel === 'dom' ? <pre className="browser-dom-output">{dom || '页面尚未产生 DOM 证据。'}</pre> : null}
      {panel === 'screenshot' ? screenshot ? <img className="browser-screenshot" src={screenshot} alt="当前受控页面截图"/> : <p>页面尚未产生截图证据。</p> : null}
      {panel === 'console' ? <div className="browser-event-list">{consoleEntries.length === 0 ? <p>没有控制台事件。</p> : consoleEntries.map((entry, index) => <article key={`${entry.timestamp}-${index}`}><span className={entry.level === 'error' ? 'error' : ''}>{entry.level}</span><code>{entry.text}</code><time>{new Date(entry.timestamp).toLocaleTimeString('zh-CN')}</time></article>)}</div> : null}
      {panel === 'network' ? <div className="browser-event-list">{networkEntries.length === 0 ? <p>没有网络响应事件。</p> : networkEntries.map((entry, index) => <article key={`${entry.timestamp}-${index}`}><span>{entry.status}</span><code>{entry.method || '响应'} {entry.url}</code><small>{entry.mime_type}</small></article>)}</div> : null}
    </div>}
  </section>
}
