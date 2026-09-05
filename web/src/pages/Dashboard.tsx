import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { HostApi, RunApi, AuditApi, DoctorApi } from '@/api/endpoints'
import { useAsync } from '@/lib/hooks'
import { Stat, Loading, ErrorBox, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { shortId, fmtRelative } from '@/lib/format'
import { rollupRuns } from '@/lib/runmath'

export function DashboardPage() {
  const hosts = useAsync(() => HostApi.list(), [], { intervalMs: 15_000 })
  const runs = useAsync(() => RunApi.list(8), [], { intervalMs: 5_000 })
  const audit = useAsync(() => AuditApi.verify(), [])
  const doctor = useAsync(() => DoctorApi.run(), [])

  const totals = useMemo(() => rollupRuns(runs.data?.runs ?? []), [runs.data])

  return (
    <>
      <div className="topbar"><h1>仪表盘</h1></div>
      <div className="content">
        <ErrorBox message={hosts.error || runs.error} />
        <div className="stat-grid" style={{ marginBottom: 16 }}>
          <Stat value={hosts.data?.hosts.length ?? '–'} label="纳管主机" />
          <Stat value={totals.running} label="执行中目标" />
          <Stat value={totals.success} label="近期成功" />
          <Stat value={totals.failed} label="近期失败" />
          <Stat value={totals.lost} label="失联目标" />
        </div>

        <div className="split">
          <div className="panel">
            <PageTitle title="最近执行" extra={<Link to="/runs">全部</Link>} />
            {runs.loading ? <Loading /> : (
              <table className="grid">
                <thead>
                  <tr><th>运行</th><th>状态</th><th>成功/失败</th><th>触发</th><th>时间</th></tr>
                </thead>
                <tbody>
                  {(runs.data?.runs ?? []).map((r) => (
                    <tr key={r.id}>
                      <td className="mono"><Link to={`/runs/${r.id}`}>{shortId(r.id)}</Link></td>
                      <td><StatusBadge status={r.status} /></td>
                      <td>{r.summary.success}/{r.summary.failed + r.summary.lost}</td>
                      <td>{r.trigger}</td>
                      <td className="muted">{fmtRelative(r.startedAt || r.createdAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <div className="col">
            <div className="panel">
              <PageTitle title="系统自检" />
              {doctor.loading ? <Loading /> : (
                <div className="kv">
                  <div className="row" style={{ gridColumn: '1/-1' }}>
                    <span className="k" style={{ width: 140 }}>总体</span>
                    <StatusBadge status={doctor.data?.ok ? 'success' : 'failed'}
                      label={doctor.data?.ok ? '健康' : '异常'} />
                  </div>
                  {(doctor.data?.checks ?? []).map((c) => (
                    <div className="row" key={c.name} style={{ gridColumn: '1/-1' }}>
                      <span className="k" style={{ width: 140 }}>{c.name}</span>
                      <span>{c.ok ? '✓' : '✗'} {c.detail}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className="panel">
              <PageTitle title="审计哈希链" extra={<Link to="/audit">查看</Link>} />
              {audit.loading ? <Loading /> : audit.data ? (
                <div className="kv">
                  <div className="row" style={{ gridColumn: '1/-1' }}>
                    <span className="k" style={{ width: 140 }}>完整性</span>
                    <StatusBadge status={audit.data.chain.ok ? 'success' : 'failed'}
                      label={audit.data.chain.ok ? `已校验 ${audit.data.chain.checked} 条` : '链断裂'} />
                  </div>
                  {audit.data.chain.reason && (
                    <div className="err-text" style={{ gridColumn: '1/-1' }}>{audit.data.chain.reason}</div>
                  )}
                </div>
              ) : null}
            </div>
          </div>
        </div>
      </div>
    </>
  )
}
