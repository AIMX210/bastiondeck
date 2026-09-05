import { useState } from 'react'
import { AuditApi } from '@/api/endpoints'
import { useAsync, useDebouncedValue } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { ErrorBox, Loading, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { download, fmtTime } from '@/lib/format'

export function AuditPage() {
  const toast = useToast()
  const [action, setAction] = useState('')
  const [actor, setActor] = useState('')
  const [result, setResult] = useState('')
  const dAction = useDebouncedValue(action)
  const dActor = useDebouncedValue(actor)
  const verify = useAsync(() => AuditApi.verify(), [])
  const list = useAsync(() => AuditApi.list({ action: dAction, actor: dActor, result, limit: 100 }),
    [dAction, dActor, result], { intervalMs: 10_000 })

  async function doExport() {
    try {
      const resp = await fetch('/api/audit/export', { credentials: 'include' })
      if (!resp.ok) {
        let msg = `导出失败（HTTP ${resp.status}）`
        try {
          const j = await resp.json()
          if (j?.error?.message) msg = j.error.message
        } catch { /* non-json body */ }
        toast.error(msg)
        return
      }
      download('audit-export.json', await resp.blob(), 'application/json')
      toast.info('已导出')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '导出失败')
    }
  }

  return (
    <>
      <div className="topbar"><h1>审计日志（哈希链防篡改）</h1></div>
      <div className="content">
        <ErrorBox message={list.error} />
        <div className="panel">
          <div className="row">
            <strong>链完整性：</strong>
            {verify.loading ? <Loading /> : verify.data && (
              <StatusBadge
                status={verify.data.chain.ok ? 'success' : 'failed'}
                label={verify.data.chain.ok ? `完整（${verify.data.chain.checked} 条）` : `断裂于 #${verify.data.chain.brokenAt}`}
              />
            )}
            <div className="grow" />
            <button onClick={() => verify.reload()}>重新校验</button>
            <button onClick={doExport}>导出 JSON</button>
          </div>
          {verify.data?.chain.reason && <div className="err-text" style={{ marginTop: 8 }}>{verify.data.chain.reason}</div>}
        </div>
        <PageTitle title="事件流" extra={
          <div className="toolbar" style={{ margin: 0 }}>
            <input placeholder="动作" value={action} onChange={(e) => setAction(e.target.value)} style={{ width: 150 }} />
            <input placeholder="操作者" value={actor} onChange={(e) => setActor(e.target.value)} style={{ width: 150 }} />
            <select value={result} onChange={(e) => setResult(e.target.value)} style={{ width: 130 }}>
              <option value="">全部结果</option>
              <option value="success">成功</option>
              <option value="denied">拒绝</option>
              <option value="failure">失败</option>
            </select>
          </div>
        } />
        {list.loading ? <Loading /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead><tr><th>#</th><th>时间</th><th>操作者</th><th>动作</th><th>对象</th><th>结果</th><th>IP</th><th>哈希</th></tr></thead>
              <tbody>
                {(list.data?.entries ?? []).map((e) => (
                  <tr key={e.eventId}>
                    <td className="mono muted">{e.seq}</td>
                    <td className="muted">{fmtTime(e.at)}</td>
                    <td>{e.actorName ?? e.actorId ?? '—'}</td>
                    <td className="mono">{e.action}</td>
                    <td className="mono muted">{e.objectType}{e.objectId ? `:${e.objectId.slice(0, 8)}` : ''}</td>
                    <td><StatusBadge status={e.result} /></td>
                    <td className="muted">{e.ip ?? '—'}</td>
                    <td className="mono muted">{e.hash.slice(0, 10)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  )
}
