import { useState } from 'react'
import { Link } from 'react-router-dom'
import { RunApi } from '@/api/endpoints'
import { useAsync, useEventSource } from '@/lib/hooks'
import { Empty, ErrorBox, Loading, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { ConfirmButton } from '@/components/Confirm'
import { fmtRelative, shortId } from '@/lib/format'

const FILTERS = ['', 'running', 'success', 'failed', 'lost', 'timeout', 'cancelled']

export function RunsPage() {
  const [filter, setFilter] = useState('')
  const runs = useAsync(() => RunApi.list(100), [], { intervalMs: 5_000 })
  useEventSource(() => runs.reload())

  const list = (runs.data?.runs ?? []).filter((r) => !filter || r.status === filter)

  return (
    <>
      <div className="topbar"><h1>任务与执行</h1></div>
      <div className="content">
        <ErrorBox message={runs.error} />
        <PageTitle title="执行记录" extra={
          <div className="pill-tabs" style={{ margin: 0 }}>
            {FILTERS.map((f) => (
              <button key={f || 'all'} className={filter === f ? 'active' : ''}
                onClick={() => setFilter(f)}>{f || '全部'}</button>
            ))}
          </div>
        } />
        {runs.loading ? <Loading /> : list.length === 0 ? <Empty text="暂无执行记录" /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead>
                <tr><th>运行 ID</th><th>状态</th><th>目标</th><th>成/败/失</th><th>触发</th><th>开始</th><th></th></tr>
              </thead>
              <tbody>
                {list.map((r) => (
                  <tr key={r.id}>
                    <td className="mono"><Link to={`/runs/${r.id}`}>{shortId(r.id)}</Link></td>
                    <td><StatusBadge status={r.status} /></td>
                    <td>{r.summary.total}</td>
                    <td>{r.summary.success}/{r.summary.failed}/{r.summary.lost + r.summary.timeout}</td>
                    <td>{r.trigger}</td>
                    <td className="muted">{fmtRelative(r.startedAt || r.createdAt)}</td>
                    <td>
                      {['pending', 'running'].includes(r.status) && (
                        <ConfirmButton message="取消本次运行？已在远端执行的命令会收到中断信号"
                          onConfirm={async () => { await RunApi.cancel(r.id); runs.reload() }}>
                          <button className="sm danger">取消</button>
                        </ConfirmButton>
                      )}
                    </td>
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
