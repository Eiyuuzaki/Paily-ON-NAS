/**
 * 复制文本到剪贴板。
 *
 * 优先使用 Clipboard API —— 但它只在「安全上下文」（HTTPS 或 localhost）可用。
 * 内网用 http://<NAS-IP>:21381 访问时 navigator.clipboard 不存在，
 * 此时退回 execCommand('copy')，保证 http 环境下也能复制。
 *
 * @returns 是否复制成功
 */
export async function copyText(text: string): Promise<boolean> {
  // 首选：Clipboard API（HTTPS / localhost）
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    /* 落到下面的降级方案 */
  }

  // 降级：临时 textarea + execCommand（http 内网可用）
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.top = '-1000px'
    ta.style.left = '-1000px'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    ta.setSelectionRange(0, text.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
