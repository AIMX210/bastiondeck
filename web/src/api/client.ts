// Minimal typed fetch wrapper. Cookie sessions send X-BDK-CSRF on writes;
// native bearer tokens are supported for parity with the CLI.

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

type Envelope<T> = { data: T } | { error: { code: string; message: string } }

let tokenStore: string | null = null
export const setToken = (t: string | null) => { tokenStore = t }
export const getToken = () => tokenStore

async function request<T>(method: string, path: string, body?: unknown, query?: Record<string, string|number|undefined>): Promise<T> {
  let url = path
  if (query) {
    const params = new URLSearchParams()
    Object.entries(query).forEach(([k, v]) => { if (v !== undefined && v !== '') params.set(k, String(v)) })
    const qs = params.toString()
    if (qs) url += `?${qs}`
  }
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (method !== 'GET' && method !== 'HEAD') headers['X-BDK-CSRF'] = '1'
  if (tokenStore) headers['Authorization'] = `Bearer ${tokenStore}`
  const resp = await fetch(url, {
    method,
    headers,
    credentials: 'include',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const text = await resp.text()
  let parsed: Envelope<T> | null = null
  if (text) {
    try { parsed = JSON.parse(text) } catch { /* non-json */ }
  }
  if (!resp.ok) {
    const err = parsed && 'error' in parsed ? parsed.error : null
    throw new ApiError(resp.status, err?.code ?? 'http_error', err?.message ?? resp.statusText)
  }
  if (parsed && 'data' in parsed) return parsed.data as T
  return undefined as unknown as T
}

export const api = {
  get: <T>(p: string, q?: Record<string, string|number|undefined>) => request<T>('GET', p, undefined, q),
  post: <T>(p: string, b?: unknown) => request<T>('POST', p, b ?? {}),
  patch: <T>(p: string, b?: unknown) => request<T>('PATCH', p, b),
  put: <T>(p: string, b?: unknown) => request<T>('PUT', p, b),
  del: <T>(p: string) => request<T>('DELETE', p),
}

// Build a WebSocket URL for the current origin.
export function wsUrl(path: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}${path}`
}
