const HOST = 'dev.aether.desktop'

async function snapshotActiveTab() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (!tab?.id || !tab.url || !/^https?:|^file:/.test(tab.url)) {
    throw new Error('当前标签页不支持结构化快照')
  }
  const [{ result }] = await chrome.scripting.executeScript({
    target: { tabId: tab.id },
    func: () => ({
      title: document.title,
      url: location.href,
      dom: document.documentElement.outerHTML.slice(0, 900_000),
    }),
  })
  return { type: 'snapshot', id: crypto.randomUUID(), tab_id: tab.id, payload: result }
}

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type !== 'capture_current_tab') return false
  snapshotActiveTab()
    .then((snapshot) => chrome.runtime.sendNativeMessage(HOST, snapshot))
    .then((response) => sendResponse({ ok: true, response }))
    .catch((error) => sendResponse({ ok: false, error: String(error) }))
  return true
})
