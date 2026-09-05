// Formatting helpers shared across pages.

export function shortId(id?: string, n = 10): string {
  if (!id) return '—'
  return id.length > n ? id.slice(0, n) : id
}

export function fmtTime(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

export function fmtRelative(iso?: string): string {
  if (!iso) return '—'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return iso
  const diff = Date.now() - then
  const s = Math.floor(diff / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

export function fmtBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function fmtUptime(sec: number): string {
  if (!sec || sec < 0) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

export function modeString(mode: number): string {
  // POSIX rwxrwxrwx
  const bits = ['x', 'w', 'r']
  let out = ''
  for (let shift = 8; shift >= 0; shift--) {
    const on = (mode >> shift) & 1
    const ch = bits[shift % 3]
    out += on ? ch : '-'
  }
  return out
}

export const STATUS_LABEL: Record<string, string> = {
  pending: '等待', running: '执行中', success: '成功', failed: '失败',
  timeout: '超时', cancelled: '已取消', lost: '失联', skipped: '跳过',
}

export function statusLabel(s: string): string {
  return STATUS_LABEL[s] ?? s
}

export function download(filename: string, content: BlobPart, mime = 'application/octet-stream') {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

export function classNames(...xs: (string | false | null | undefined)[]): string {
  return xs.filter(Boolean).join(' ')
}
