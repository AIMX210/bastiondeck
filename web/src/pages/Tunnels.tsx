import { useState } from 'react'
import { HostApi, TunnelApi } from '@/api/endpoints'
import type { Tunnel } from '@/api/types'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { useAuth } from '@/store/auth'
import { Empty, ErrorBox, Loading, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { Modal } from '@/components/Modal'
import { ConfirmButton } from '@/components/Confirm'
import { fmtTime } from '@/lib/format'

export function TunnelsPage() {
  const toast = useToast()
  const { can } = useAuth()
  const list = useAsync(() => TunnelApi.list(), [], { intervalMs: 5_000 })
  const hosts = useAsync(() => HostApi.list(), [])
  const [form, setForm] = useState<Partial<Tunnel> | null>(null)

  async function create() {
    if (!form) return
    if (!form.hostId) { toast.error('请选择主机'); return }
    const lp = Number(form.localPort), rp = Number(form.remotePort)
    if (!lp || lp < 1 || lp > 65535) { toast.error('本地端口需在 1-65535'); return }
    if (!rp || rp < 1 || rp > 65535) { toast.error('远端端口需在 1-65535'); return }
    try {
      await TunnelApi.create({
        hostId: form.hostId, kind: form.kind ?? 'local',
        localHost: form.localHost || '127.0.0.1', localPort: lp,
        remoteHost: form.remoteHost || '127.0.0.1', remotePort: rp,
      })
      toast.success('隧道已建立')
      setForm(null); list.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '建立失败') }
  }

  return (
    <>
      <div className="topbar"><h1>端口转发</h1></div>
      <div className="content">
        <ErrorBox message={list.error} />
        <PageTitle title="SSH 隧道（守护进程托管，重启后自动对账恢复）" extra={
          can('exec') && (
            <button className="primary" onClick={() => setForm({ kind: 'local', localHost: '127.0.0.1', remoteHost: '127.0.0.1' })}>+ 新建隧道</button>
          )
        } />
        {list.loading ? <Loading /> : (list.data?.tunnels.length === 0) ? <Empty text="暂无隧道" /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead><tr><th>类型</th><th>主机</th><th>本地</th><th>远端</th><th>状态</th><th>启动时间</th><th></th></tr></thead>
              <tbody>
                {(list.data?.tunnels ?? []).map((t) => {
                  const host = hosts.data?.hosts.find((h) => h.id === t.hostId)
                  const live = t.status === 'active' || t.status === 'starting'
                  return (
                    <tr key={t.id}>
                      <td>{t.kind === 'local' ? '本地转发 -L' : '远程转发 -R'}</td>
                      <td>{host?.name ?? t.hostId}</td>
                      <td className="mono">{t.localHost}:{t.localPort}</td>
                      <td className="mono">{t.remoteHost}:{t.remotePort}</td>
                      <td><StatusBadge status={t.status} />{t.lastError && <span className="err-text"> {t.lastError}</span>}</td>
                      <td className="muted">{fmtTime(t.startedAt)}</td>
                      <td>
                        {live && can('exec') && (
                          <ConfirmButton message="停止隧道？" onConfirm={async () => {
                            await TunnelApi.stop(t.id); list.reload()
                          }}><button className="sm danger">停止</button></ConfirmButton>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
        {form && (
          <Modal title="新建隧道" onClose={() => setForm(null)}
            footer={<><button onClick={() => setForm(null)}>取消</button>
              <button className="primary" onClick={create}>建立</button></>}>
            <label className="field"><span>主机</span>
              <select value={form.hostId ?? ''} onChange={(e) => setForm({ ...form, hostId: e.target.value })}>
                <option value="">请选择</option>
                {(hosts.data?.hosts ?? []).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
              </select></label>
            <label className="field"><span>类型</span>
              <select value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value as 'local'|'remote' })}>
                <option value="local">本地转发（访问本地端口 → 远端可达地址）</option>
                <option value="remote">远程转发（远端端口 → 本地可达地址）</option>
              </select></label>
            <div className="split">
              <label className="field"><span>本地监听地址</span>
                <input value={form.localHost} onChange={(e) => setForm({ ...form, localHost: e.target.value })} /></label>
              <label className="field"><span>本地端口</span>
                <input type="number" value={form.localPort ?? ''} onChange={(e) => setForm({ ...form, localPort: Number(e.target.value) })} /></label>
              <label className="field"><span>远端地址</span>
                <input value={form.remoteHost} onChange={(e) => setForm({ ...form, remoteHost: e.target.value })} /></label>
              <label className="field"><span>远端端口</span>
                <input type="number" value={form.remotePort ?? ''} onChange={(e) => setForm({ ...form, remotePort: Number(e.target.value) })} /></label>
            </div>
          </Modal>
        )}
      </div>
    </>
  )
}
