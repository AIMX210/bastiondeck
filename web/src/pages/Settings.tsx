import { useEffect, useState } from 'react'
import { DoctorApi, SettingsApi } from '@/api/endpoints'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { useAuth } from '@/store/auth'
import { ErrorBox, Loading, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'

const FIELDS: { key: string; label: string; hint: string }[] = [
  { key: 'session.ttlMinutes', label: '会话有效期（分钟）', hint: '默认 720；滑动续期，剩余不足一半时延长' },
  { key: 'audit.retentionDays', label: '审计保留天数', hint: '超期由维护任务归档' },
  { key: 'metrics.enabled', label: '启用指标采集', hint: 'true / false' },
  { key: 'metrics.intervalSec', label: '指标采集周期（秒）', hint: '后台采集循环间隔' },
  { key: 'exec.defaultTimeoutSec', label: '默认执行超时（秒）', hint: '单次命令默认上限' },
  { key: 'exec.maxConcurrency', label: '最大并发目标', hint: '扇出执行的并发闸' },
  { key: 'theme.default', label: '默认主题', hint: 'system / dark / light' },
]

export function SettingsPage() {
  const toast = useToast()
  const { can } = useAuth()
  const loaded = useAsync(() => SettingsApi.get(), [])
  const doctor = useAsync(() => can('exec') ? DoctorApi.run() : Promise.resolve(null), [])
  const [draft, setDraft] = useState<Record<string, string>>({})
  useEffect(() => { if (loaded.data) setDraft(loaded.data.settings) }, [loaded.data])

  async function save() {
    try {
      await SettingsApi.put(draft)
      toast.success('已保存'); loaded.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '保存失败') }
  }

  return (
    <>
      <div className="topbar"><h1>设置与自检</h1></div>
      <div className="content">
        <ErrorBox message={loaded.error} />
        <div className="panel">
          <PageTitle title="运行参数" extra={
            can('manage_inventory')
              ? <button className="primary" onClick={save}>保存设置</button>
              : <span className="muted">仅 admin/owner 可修改运行参数</span>
          } />
          {loaded.loading ? <Loading /> : (
            FIELDS.map((f) => (
              <label className="field" key={f.key}>
                <span>{f.label} <span className="muted">（{f.hint}）</span></span>
                <input value={draft[f.key] ?? ''} onChange={(e) => setDraft({ ...draft, [f.key]: e.target.value })} />
              </label>
            ))
          )}
        </div>

        {can('exec') && (
        <div className="panel">
          <PageTitle title="Doctor 自检" extra={
            <div className="row">
              <StatusBadge status={doctor.data ? (doctor.data.ok ? 'success' : 'failed') : 'pending'}
                label={doctor.data ? (doctor.data.ok ? '健康' : '存在异常项') : '—'} />
              <button onClick={() => doctor.reload()}>重新自检</button>
            </div>
          } />
          {doctor.loading ? <Loading /> : (
            <div className="kv">
              {(doctor.data?.checks ?? []).map((c) => (
                <div className="row" key={c.name} style={{ gridColumn: '1 / -1' }}>
                  <span className="k" style={{ width: 140 }}>{c.name}</span>
                  <span>{c.ok ? '✓' : '✗'} {c.detail ?? ''}</span>
                </div>
              ))}
            </div>
          )}
        </div>
        )}
      </div>
    </>
  )
}
