/**
 * Format an ISO date string to a readable local datetime string.
 */
export function formatDate(iso: string): string {
    if (!iso) return '—'
    const d = new Date(iso)
    if (isNaN(d.getTime())) return iso
    return d.toLocaleString('zh-CN', { hour12: false })
}

/**
 * Format latency in ms, showing decimals only when < 1000ms.
 */
export function formatLatency(ms: number): string {
    if (ms == null) return '—'
    if (ms >= 1000) return `${(ms / 1000).toFixed(2)} s`
    return `${ms} ms`
}

/**
 * Truncate a string to n characters, appending '…' if longer.
 */
export function truncate(str: string, n = 40): string {
    if (!str) return ''
    return str.length > n ? str.slice(0, n) + '…' : str
}

/**
 * Format a byte count into a human-readable string (B / KB / MB / GB / TB).
 */
export function formatBytes(bytes: number | null | undefined): string {
    if (bytes == null || bytes < 0) return '—'
    if (bytes === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
    const v = bytes / Math.pow(1024, i)
    return `${v >= 100 ? v.toFixed(0) : v.toFixed(2)} ${units[i]}`
}

/**
 * Format a unix timestamp (seconds) into a date string. 0 = 无限期.
 */
export function formatUnixDate(sec: number | null | undefined): string {
    if (!sec || sec <= 0) return '无限期'
    const d = new Date(sec * 1000)
    if (isNaN(d.getTime())) return '—'
    return d.toLocaleDateString('zh-CN')
}

/**
 * Decode a JWT payload without verifying signature.
 * Returns the exp timestamp (seconds) or null if not present / invalid.
 * For display/debugging only — never use for security decisions.
 */
export function decodeJwtExp(token: string): number | null {
    try {
        const payload = token.split('.')[1]
        if (!payload) return null
        const decoded = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/')))
        return typeof decoded.exp === 'number' ? decoded.exp : null
    } catch {
        return null
    }
}
