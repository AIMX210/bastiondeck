import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, setToken } from './client'
import { b64FromUtf8, utf8FromB64 } from './endpoints'

function mockFetchOnce(body: unknown, init?: ResponseInit) {
  globalThis.fetch = vi.fn(async () =>
    new Response(typeof body === 'string' ? body : JSON.stringify(body), init),
  ) as unknown as typeof fetch
}

afterEach(() => vi.restoreAllMocks())

describe('api envelope handling', () => {
  it('unwraps the data envelope', async () => {
    mockFetchOnce({ data: { hello: 'world' } })
    const v = await api.get<{ hello: string }>('/api/x')
    expect(v.hello).toBe('world')
  })

  it('throws ApiError with code/message on error envelope', async () => {
    mockFetchOnce({ error: { code: 'csrf_required', message: 'missing header' } }, { status: 403 })
    const err = await api.post('/api/x').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBe('csrf_required')
    expect((err as ApiError).status).toBe(403)
  })

  it('falls back to status text when body is not json', async () => {
    mockFetchOnce('boom', { status: 500, statusText: 'Internal Error' })
    const err = await api.get('/api/x').catch((e) => e)
    expect((err as ApiError).code).toBe('http_error')
    expect((err as ApiError).message).toBe('Internal Error')
  })

  it('encodes query params and skips undefined values', async () => {
    const spy = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify({ data: 1 })))
    globalThis.fetch = spy as unknown as typeof fetch
    await api.get('/api/y', { a: 1, b: '', c: undefined, d: 'x' })
    const url = spy.mock.calls[0][0]
    expect(url).toContain('a=1')
    expect(url).toContain('d=x')
    expect(url).not.toContain('b=')
    expect(url).not.toContain('c=')
  })

  it('attaches CSRF header on writes and bearer when token set', async () => {
    const spy = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify({ data: null })))
    globalThis.fetch = spy as unknown as typeof fetch
    setToken('tok')
    await api.del('/api/z')
    const headers = spy.mock.calls[0][1]!.headers as Record<string, string>
    expect(headers['X-BDK-CSRF']).toBe('1')
    expect(headers.Authorization).toBe('Bearer tok')
    setToken(null)
  })

  it('does not attach CSRF on GET', async () => {
    const spy = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify({ data: null })))
    globalThis.fetch = spy as unknown as typeof fetch
    await api.get('/api/z')
    const headers = (spy.mock.calls[0][1]!.headers) as Record<string, string>
    expect(headers['X-BDK-CSRF']).toBeUndefined()
  })
})

describe('utf8 base64 roundtrip', () => {
  it('handles multibyte Chinese', () => {
    const s = '主机舰队-🚀'
    expect(utf8FromB64(b64FromUtf8(s))).toBe(s)
  })
  it('utf8FromB64 returns empty string on garbage', () => {
    expect(utf8FromB64('@@not-base64@@')).toBe('')
  })
})
