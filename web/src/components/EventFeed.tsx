import { useEffect, useRef, useState } from 'react'
import { useEventSource } from '@/lib/hooks'
import { fmtTime } from '@/lib/format'
import { StatusBadge } from './Badge'

interface FeedItem {
  key: number
  at: string
  type: string
  runId?: string
  targetId?: string
  status?: string
  message?: string
}

let feedSeq = 0

// Global-ish live feed panel driven by the /api/events SSE stream.
export function EventFeed({ height = 320 }: { height?: number }) {
  const [items, setItems] = useState<FeedItem[]>([])
  const [paused, setPaused] = useState(false)
  const pausedRef = useRef(false)
  pausedRef.current = paused

  useEventSource((type, data) => {
    if (pausedRef.current) return
    const d = data as Record<string, unknown>
    setItems((xs) => [{
      key: ++feedSeq,
      at: new Date().toISOString(),
      type,
      runId: d.runId as string | undefined,
      targetId: d.targetId as string | undefined,
      status: d.status as string | undefined,
      message: d.message as string | undefined,
    }, ...xs].slice(0, 200))
  })

  useEffect(() => {
    const t = window.setInterval(() => setItems((xs) => xs.slice()), 60_000)
    return () => window.clearInterval(t)
  }, [])

  return (
    <div className="panel">
      <div className="toolbar" style={{ marginBottom: 8 }}>
        <h2 style={{ margin: 0 }}>实时事件流（SSE）</h2>
        <div className="grow" />
        <button className="sm" onClick={() => setPaused((p) => !p)}>{paused ? '继续' : '暂停'}</button>
        <button className="sm" onClick={() => setItems([])}>清空</button>
      </div>
      <div style={{ height, overflow: 'auto', fontFamily: 'var(--mono)', fontSize: 12.5 }}>
        {items.length === 0 && <div className="muted">等待事件…（执行任务时这里会实时滚动）</div>}
        {items.map((it) => (
          <div key={it.key} className="row" style={{ gap: 10, padding: '3px 0', borderBottom: '1px solid var(--border-soft)' }}>
            <span className="muted">{fmtTime(it.at)}</span>
            <span style={{ width: 96 }}>{it.type}</span>
            {it.status && <StatusBadge status={it.status} />}
            <span className="muted">{it.runId ?? ''}{it.targetId ? ` / ${it.targetId.slice(0, 10)}` : ''}</span>
            <span>{it.message}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
