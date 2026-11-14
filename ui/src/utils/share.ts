// 把节点 raw（clash 结构对象或 JSON 字符串）转成可导入的分享 URI
// 支持 vless / trojan（ss 的加密格式客户端兼容性差，暂不支持）
export function rawToShareUri(raw: string | Record<string, unknown>): string {
  let o: Record<string, unknown>
  if (typeof raw === 'object' && raw !== null) {
    o = raw as Record<string, unknown>
  } else {
    try { o = JSON.parse(raw as string) } catch { return '' }
  }
  const name = encodeURIComponent(String(o.name || ''))
  const frag = (u: URLSearchParams) => u.toString() ? '?' + u.toString() + '#' + name : '#' + name
  const u = new URLSearchParams()
  if (o.type === 'vless') {
    u.set('type', String(o.network || 'tcp'))
    u.set('security', o.tls ? 'tls' : 'none')
    if (o.servername) u.set('sni', String(o.servername))
    if (o['client-fingerprint']) u.set('fp', String(o['client-fingerprint']))
    const ws = o['ws-opts'] as Record<string, unknown> | undefined
    if (ws) {
      u.set('path', String(ws.path || '/'))
      const h = ws.headers as Record<string, string> | undefined
      if (h?.Host) u.set('host', h.Host)
    }
    const ech = o['ech-opts'] as Record<string, unknown> | undefined
    if (ech?.enable && ech.config) u.set('ech', String(ech.config))
    return `vless://${o.uuid}@${o.server}:${o.port}${frag(u)}`
  }
  if (o.type === 'trojan') {
    if (o.servername) u.set('sni', String(o.servername))
    if (o['client-fingerprint']) u.set('fp', String(o['client-fingerprint']))
    const ws = o['ws-opts'] as Record<string, unknown> | undefined
    if (ws) {
      u.set('type', String(o.network || 'tcp'))
      u.set('path', String(ws.path || '/'))
      const h = ws.headers as Record<string, string> | undefined
      if (h?.Host) u.set('host', h.Host)
    }
    return `trojan://${o.password}@${o.server}:${o.port}${frag(u)}`
  }
  return ''
}
