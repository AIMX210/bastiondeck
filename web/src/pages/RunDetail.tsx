import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { HostApi, RunApi } from '@/api/endpoints'
import { useAsync, useEventSource } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { ErrorBox, Loading, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { ConfirmButton } from '@/components/Confirm'
import { fmtTime, shortId } from '@/lib/format'
import type { Host, RunTarget } from '@/api/types'

export function RunDetailPage() {
  const { id = '' } = useParams()
  const toast = useToast()
  const run = useAsync(() => RunApi.get(id), [id], { intervalMs: 3_000 })
  const hosts = useAsync(() => HostApi.list(), [])
  const [selected, setSelected] = useState<string | null>(null)
  const [stream, setStream] = useState<'stdout' | 'stderr'>('stdout')
  const [text, setText] = useState('')
  const offsetRef = useRef(0)
  const live = run.data?.live

  useEventSource(() => run.reload())

  const hostName = useCallback((hid: string) => {
    const h = (hosts.data?.hosts ?? []).find((x: Host) => x.id === hid)
    return h ? h.name : shortId(hid)
  }, [hosts.data])

  const targets = run.data?.run.targets ?? []
  const active = selected ?? targets[0]?.id ?? null

  // Tail the selected target output, honoring stdout/stderr separation.
  useEffect(() => {
    setText(''); offsetRef.current = 0
  }, [active, stream])

  useEffect(() => {
    if (!active) return
    let stop = false
    async function pull() {
      if (stop) return
      try {
        const r = await RunApi.output(id, active, stream, offsetRef.current)
        if (!stop && r.chunk) {
          setText((t) => t + r.chunk)
          offsetRef.current = r.offset
        }
      } catch { /* transient */ }
    }
    pull()
    const t = window.setInterval(pull, 1500)
    return () => { stop = true; window.clearInterval(t) }
  }, [id, active, stream])

  if (run.loading && !run.data) return <div className="content"><Loading /></div>
  const r = run.data?.run
  if (!r) return <div className="content"><ErrorBox message={run.error ?? '运行不存在'} /></div>

  return (
    <>
      <div className="topbar">
        <h1><Link to="/runs">执行</Link> / <span className="mono">{shortId(id, 16)}</span></h1>
        <StatusBadge status={r.status} />
      </div>
      <div className="content">
        <ErrorBox message={run.error} />
        <div className="panel">
          <div className="kv">
            <div className="row" style={{ gridColumn: '1/-1' }}>
              <span className="k" style={{ width: 140 }}>开始/结束</span>
              <span>{fmtTime(r.startedAt)} → {fmtTime(r.endedAt)}</span>
            </div>
            <div className="row" style={{ gridColumn: '1/-1' }}>
              <span className="k" style={{ width: 140 }}>聚合</span>
              <span>
                共 {r.summary.total}，成功 {r.summary.success}，失败 {r.summary.failed}，
                超时 {r.summary.timeout}，失联 {r.summary.lost}，取消 {r.summary.cancelled}
              </span>
            </div>
          </div>
          <div className="toolbar" style={{ marginBottom: 0, marginTop: 10 }}>
            {live && (
              <ConfirmButton message="取消本次运行？" onConfirm={async () => {
                await RunApi.cancel(id); run.reload(); toast.info('已请求取消')
              }}><button className="danger">急停</button></ConfirmButton>
            )}
            {!live && r.summary.failed + r.summary.lost > 0 && (
              <button onClick={async () => {
                const nr = await RunApi.retry(id); toast.success('已重投失败目标'); window.location.hash = `#/runs/${nr.runId}`
              }}>仅重试失败/失联</button>
            )}
          </div>
        </div>

        <div className="split" style={{ gridTemplateColumns: '340px 1fr' }}>
          <div className="panel" style={{ padding: 0 }}>
            <table className="grid">
              <thead><tr><th>目标</th><th>状态</th><th>退出码</th></tr></thead>
              <tbody>
                {targets.map((t: RunTarget) => (
                  <tr key={t.id} className={t.id === active ? 'selected' : ''}
                    style={{ cursor: 'pointer' }} onClick={() => setSelected(t.id)}>
                    <td>{hostName(t.hostId)}</td>
                    <td><StatusBadge status={t.status} /></td>
                    <td className="mono">{t.exitCode ?? '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="panel">
            <div className="toolbar" style={{ marginBottom: 8 }}>
              <div className="pill-tabs" style={{ margin: 0 }}>
                <button className={stream === 'stdout' ? 'active' : ''} onClick={() => setStream('stdout')}>stdout</button>
                <button className={stream === 'stderr' ? 'active' : ''} onClick={() => setStream('stderr')}>stderr</button>
              </div>
              <div className="grow" />
              {targets.find((t) => t.id === active)?.errorText &&
                <span className="err-text">{targets.find((t) => t.id === active)?.errorCode}: {targets.find((t) => t.id === active)?.errorText}</span>}
            </div>
            <pre className="mono" style={{
              background: '#0a0e12', borderRadius: 8, padding: 12, minHeight: 360,
              maxHeight: '60vh', overflow: 'auto', whiteSpace: 'pre-wrap', margin: 0,
            }}>{text || '（暂无输出）'}</pre>
          </div>
        </div>
      </div>
    </>
  )
}
