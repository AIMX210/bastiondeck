import { useCallback, useEffect, useRef, useState } from 'react'

// useAsync loads data on mount and whenever deps change.
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[] = [], opts?: { intervalMs?: number }) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const fnRef = useRef(fn)
  fnRef.current = fn

  const reload = useCallback(async () => {
    try {
      const v = await fnRef.current()
      setData(v)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    reload()
    if (!opts?.intervalMs) return
    const t = window.setInterval(reload, opts.intervalMs)
    return () => window.clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reload, ...deps])

  return { data, error, loading, reload, setData }
}

// useNow re-renders on an interval for relative timestamps.
export function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(t)
  }, [intervalMs])
  return now
}

// useEventSource subscribes to the SSE event stream.
export function useEventSource(onEvent: (type: string, data: unknown) => void, enabled = true) {
  const cbRef = useRef(onEvent)
  cbRef.current = onEvent
  useEffect(() => {
    if (!enabled) return
    const es = new EventSource('/api/events', { withCredentials: true })
    const handler = (e: MessageEvent) => {
      try { cbRef.current(e.type, JSON.parse(e.data)) } catch { /* ignore */ }
    }
    const types = ['run_update', 'target_update', 'message']
    types.forEach((t) => es.addEventListener(t, handler as EventListener))
    return () => es.close()
  }, [enabled])
}

// useDebouncedValue delays propagating a fast-changing value.
export function useDebouncedValue<T>(value: T, delay = 250): T {
  const [v, setV] = useState(value)
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), delay)
    return () => window.clearTimeout(t)
  }, [value, delay])
  return v
}
