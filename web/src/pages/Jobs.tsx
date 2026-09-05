import { useState } from 'react'
import { ExecApi, HostApi, JobApi, SnippetApi } from '@/api/endpoints'
import type { Job } from '@/api/types'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { Empty, ErrorBox, Loading, PageTitle } from '@/components/Common'
import { Modal } from '@/components/Modal'
import { ConfirmButton } from '@/components/Confirm'
import { fmtRelative, shortId } from '@/lib/format'

export function JobsPage() {
  const toast = useToast()
  const jobs = useAsync(() => JobApi.list(), [], { intervalMs: 10_000 })
  const hosts = useAsync(() => HostApi.list(), [])
  const snippets = useAsync(() => SnippetApi.list(), [])
  const [editing, setEditing] = useState<Partial<Job> | null>(null)

  async function save() {
    if (!editing) return
    try {
      if (editing.id) await JobApi.update(editing.id, editing)
      else await JobApi.create(editing)
      toast.success('已保存')
      setEditing(null)
      jobs.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '保存失败') }
  }

  return (
    <>
      <div className="topbar"><h1>计划任务</h1></div>
      <div className="content">
        <ErrorBox message={jobs.error} />
        <PageTitle title="Cron 计划任务（cron-lite，5 段表达式）" extra={
          <button className="primary" onClick={() => setEditing({
            kind: 'scheduled', name: '', command: '', targetIds: [],
            scheduleExpr: '*/5 * * * *', enabled: true, timeoutMs: 60_000, concurrency: 4,
          })}>+ 新建计划任务</button>
        } />
        {jobs.loading ? <Loading /> : (jobs.data?.jobs.length === 0) ? <Empty text="暂无计划任务" /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead>
                <tr><th>名称</th><th>表达式</th><th>命令</th><th>目标数</th><th>启用</th><th>上次/下次</th><th style={{ width: 220 }}>操作</th></tr>
              </thead>
              <tbody>
                {(jobs.data?.jobs ?? []).map((j) => (
                  <tr key={j.id}>
                    <td>{j.name}</td>
                    <td className="mono">{j.scheduleExpr}</td>
                    <td className="mono" style={{ maxWidth: 260, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{j.command}</td>
                    <td>{j.targetIds.length}</td>
                    <td>{j.enabled ? '✓' : '—'}</td>
                    <td className="muted">{fmtRelative(j.lastRunAt)} / {fmtRelative(j.nextRunAt)}</td>
                    <td>
                      <div className="row">
                        <button className="sm" onClick={async () => {
                          const r = await ExecApi.runJob(j.id); toast.success(`已手动触发 ${shortId(r.runId)}`)
                        }}>立即跑</button>
                        <button className="sm" onClick={() => setEditing(j)}>编辑</button>
                        <ConfirmButton danger message="删除该计划任务？历史执行记录保留" onConfirm={async () => {
                          await JobApi.remove(j.id); jobs.reload()
                        }}><button className="sm danger">删</button></ConfirmButton>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {editing && (
          <Modal title={editing.id ? '编辑计划任务' : '新建计划任务'} onClose={() => setEditing(null)} width={640}
            footer={<><button onClick={() => setEditing(null)}>取消</button>
              <button className="primary" onClick={save}>保存</button></>}>
            <label className="field"><span>名称</span>
              <input value={editing.name ?? ''} onChange={(e) => setEditing({ ...editing, name: e.target.value })} /></label>
            <label className="field"><span>命令片段</span>
              <select value={editing.snippetId ?? ''} onChange={(e) => {
                const s = snippets.data?.snippets.find((x) => x.id === e.target.value)
                setEditing({ ...editing, snippetId: e.target.value || undefined, command: s?.body ?? editing.command })
              }}>
                <option value="">自定义命令</option>
                {(snippets.data?.snippets ?? []).map((s) => <option key={s.id} value={s.id}>{s.title}</option>)}
              </select></label>
            <label className="field"><span>命令</span>
              <textarea rows={4} className="mono" value={editing.command ?? ''}
                onChange={(e) => setEditing({ ...editing, command: e.target.value })} /></label>
            <label className="field"><span>Cron 表达式（分 时 日 月 周）</span>
              <input className="mono" value={editing.scheduleExpr ?? ''}
                onChange={(e) => setEditing({ ...editing, scheduleExpr: e.target.value })} /></label>
            <div className="split">
              <label className="field"><span>超时（秒）</span>
                <input type="number" value={(editing.timeoutMs ?? 60000) / 1000}
                  onChange={(e) => setEditing({ ...editing, timeoutMs: Number(e.target.value) * 1000 })} /></label>
              <label className="field"><span>并发度</span>
                <input type="number" value={editing.concurrency ?? 4}
                  onChange={(e) => setEditing({ ...editing, concurrency: Number(e.target.value) })} /></label>
            </div>
            <label className="field"><span>目标主机（按住 Ctrl 多选）</span>
              <select multiple size={6} value={editing.targetIds ?? []}
                onChange={(e) => setEditing({
                  ...editing,
                  targetIds: Array.from(e.target.selectedOptions).map((o) => o.value),
                })}>
                {(hosts.data?.hosts ?? []).map((h) => <option key={h.id} value={h.id}>{h.name}（{h.address}）</option>)}
              </select></label>
            <label className="field row">
              <input type="checkbox" style={{ width: 'auto' }} checked={editing.enabled ?? true}
                onChange={(e) => setEditing({ ...editing, enabled: e.target.checked })} />
              <span style={{ margin: 0 }}>启用</span>
            </label>
          </Modal>
        )}
      </div>
    </>
  )
}
