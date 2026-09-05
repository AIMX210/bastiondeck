import type { ReactNode } from 'react'

export function Empty({ text, action }: { text: string; action?: ReactNode }) {
  return (
    <div className="empty">
      <div style={{ marginBottom: 10 }}>{text}</div>
      {action}
    </div>
  )
}

export function Loading({ label = '加载中…' }: { label?: string }) {
  return <div className="empty muted">{label}</div>
}

export function ErrorBox({ message }: { message?: string | null }) {
  if (!message) return null
  return <div className="panel" style={{ borderColor: 'rgba(248,113,113,.5)' }}>
    <span className="err-text">⚠ {message}</span>
  </div>
}

export function PageTitle({ title, extra }: { title: ReactNode; extra?: ReactNode }) {
  return (
    <div className="toolbar">
      <h2 style={{ margin: 0 }}>{title}</h2>
      <div className="grow" />
      {extra}
    </div>
  )
}

export function Stat({ value, label }: { value: ReactNode; label: string }) {
  return (
    <div className="stat">
      <div className="n">{value}</div>
      <div className="l">{label}</div>
    </div>
  )
}

// Sparkline renders a tiny bar series without a charting dependency.
export function Sparkline({ values, color }: { values: number[]; color?: string }) {
  const max = Math.max(1, ...values)
  return (
    <div className="spark" aria-label="sparkline">
      {values.map((v, i) => (
        <i
          key={i}
          style={{
            height: `${Math.max(4, (v / max) * 100)}%`,
            background: color ?? undefined,
          }}
        />
      ))}
    </div>
  )
}

export function KeyVal({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <div className="kv">
      {rows.map(([k, v]) => (
        <div key={k} className="row" style={{ gridColumn: '1 / -1' }}>
          <span className="k" style={{ width: 140 }}>{k}</span>
          <span>{v ?? '—'}</span>
        </div>
      ))}
    </div>
  )
}
