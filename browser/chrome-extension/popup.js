const button = document.querySelector('#capture')
const status = document.querySelector('#status')
button.addEventListener('click', () => {
  button.disabled = true
  status.textContent = '正在采集当前标签页…'
  chrome.runtime.sendMessage({ type: 'capture_current_tab' }, (response) => {
    const error = chrome.runtime.lastError?.message || response?.error
    status.textContent = error ? `发送失败：${error}` : '已发送到本机 Aether Desktop'
    button.disabled = false
  })
})
