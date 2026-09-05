import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ExecApi, HostApi, MetricsApi } from '@/api/endpoints'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { useAuth } from '@/store/auth'
import { useNavigate } from 'react-router-dom'
import { ErrorBox, KeyVal, Loading, PageTitle, Sparkline, Stat } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { ConfirmButton } from '@/components/Confirm'
import { TerminalPane } from '@/components/Terminal'
import { fmtBytes, fmtUptime, shortId } from '@/lib/format'
import type { MetricPoint } from '@/api/types'

export function HostDetailPage() {
  const { id = '' } = useParams()
  const toast = useToast()
  const navigate = useNavigate()
  const { can } = useAuth()
  const canExec = can('exec')
  const canManage = can('manage_inventory')
  const [tab, setTab] = useState<'term' | 'facts' | 'metrics' | 'exec'>(canExec ? 'term' : 'facts')
  const host = useAsync(() => HostApi.get(id), [id])
  const facts = useAsync(
    () => canExec ? HostApi.facts(id).then((r) => r.facts).catch(() => null) : Promise.resolve(null),
    [id, canExec],
  )
  const metrics = useAsync(() => MetricsApi.host(id), [id], { intervalMs: 15_000 })
  const [cmd, setCmd] = useState('uname -a')
  const [runId, setRunId] = useState<string | null>(null)

  async function quickExec() {
    try {
      const r = await ExecApi.run({ command: cmd, targetIds: [id], timeoutSec: 30 })
      setRunId(r.runId)
      toast.success('已发起执行')
      navigate(`/runs/${r.runId}`)
    } catch (e) { toast.error(e instanceof Error ? e.message : '执行失败') }
  }

  if (host.loading) return <div className="content"><Loading /></div>
  const h = host.data?.host
  if (!h) return <div className="content"><ErrorBox message={host.error ?? '主机不存在'} /></div>

  const cpu: MetricPoint[] = ((metrics.data?.series?.cpu as MetricPoint[]) ?? [])
  const mem: MetricPoint[] = ((metrics.data?.series?.mem as MetricPoint[]) ?? [])

  return (
    <>
      <div className="topbar">
        <h1><Link to="/hosts">主机</Link> / {h.name}</h1>
        <StatusBadge status={h.lastStatus || 'pending'} label={h.lastStatus || '未探测'} />
      </div>
      <div className="content">
        <ErrorBox message={host.error} />
        <div className="panel">
          <KeyVal rows={[
            ['ID', <span className="mono">{h.id}</span>],
            ['地址', <span className="mono">{h.address}:{h.port}</span>],
            ['用户名', h.username],
            ['认证方式', h.authKind],
            ['凭据', h.credentialId ? shortId(h.credentialId) : '—'],
            ['跳板', h.jumpHostId ? shortId(h.jumpHostId) : '直连'],
            ['标签', (h.tags ?? []).join(', ') || '—'],
            ['已知主机密钥', <span className="mono">{h.knownHostKey ? `${h.knownHostKeyType ?? 'ssh'} ${h.knownHostKey}` : '尚未记录（首次连接走 TOFU）'}</span>],
            ['最近连通', h.lastConnectedAt ?? '—'],
            ['备注', h.notes || '—'],
          ]} />
          <div className="toolbar" style={{ marginTop: 12, marginBottom: 0 }}>
            <button onClick={() => host.reload()}>刷新</button>
            {canManage && (
              <button onClick={async () => {
                try { await HostApi.resetKey(id); toast.success('已重置，下次连接重新信任'); host.reload() }
                catch (e) { toast.error(e instanceof Error ? e.message : '失败') }
              }}>重置主机密钥</button>
            )}
            <div className="grow" />
            {canManage && (
              <ConfirmButton danger message={`删除主机 ${h.name}？`} onConfirm={async () => {
                await HostApi.remove(id); navigate('/hosts')
              }}><button className="sm danger">删除</button></ConfirmButton>
            )}
          </div>
        </div>

        <div className="pill-tabs">
          {([
            ...(canExec ? ([['term', '交互终端'], ['exec', '快捷执行']] as const) : []),
            ['facts', '主机信息'] as const,
            ['metrics', '指标'] as const,
          ]).map(
            ([k, l]) => <button key={k} className={tab === k ? 'active' : ''} onClick={() => setTab(k)}>{l}</button>)}
        </div>

        {tab === 'term' && <TerminalPane hostId={id} />}

        {tab === 'exec' && (
          <div className="panel">
            <label className="field"><span>命令</span>
              <input value={cmd} onChange={(e) => setCmd(e.target.value)} /></label>
            <div className="row">
              <button className="primary" onClick={quickExec}>执行并跳转结果</button>
              {runId && <Link to={`/runs/${runId}`}>最近一次：{shortId(runId)}</Link>}
            </div>
          </div>
        )}

        {tab === 'facts' && (
          <div className="panel">
            {facts.loading ? <Loading /> : facts.data ? (
              <div className="stat-grid" style={{ marginBottom: 14 }}>
                <Stat value={facts.data.hostname} label="主机名" />
                <Stat value={`${facts.data.os}/${facts.data.arch}`} label="系统/架构" />
                <Stat value={facts.data.kernel} label="内核" />
                <Stat value={facts.data.cpuCores} label="CPU 核数" />
                <Stat value={fmtBytes(facts.data.memTotal)} label="内存" />
                <Stat value={fmtUptime(facts.data.uptimeSec)} label="运行时长" />
              </div>
            ) : <div className="empty muted">无法获取（主机离线或 agent 未连接）</div>}
          </div>
        )}

        {tab === 'metrics' && (
          <div className="col">
            <div className="panel">
              <h2>CPU（近 15 分钟）</h2>
              {cpu.length ? <Sparkline values={cpu.map((p) => p.value)} /> : <span className="muted">暂无数据</span>}
            </div>
            <div className="panel">
              <h2>内存（近 15 分钟）</h2>
              {mem.length ? <Sparkline values={mem.map((p) => p.value)} /> : <span className="muted">暂无数据</span>}
            </div>
          </div>
        )}
      </div>
    </>
  )
}
