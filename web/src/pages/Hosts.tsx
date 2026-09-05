import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { CredApi, HostApi } from '@/api/endpoints'
import type { Credential, Host } from '@/api/types'
import { useAsync, useDebouncedValue } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { Empty, ErrorBox, Loading, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { Modal } from '@/components/Modal'
import { ConfirmButton } from '@/components/Confirm'
import { GroupsPanel } from '@/components/GroupsPanel'
import { shortId } from '@/lib/format'

interface HostForm {
  name: string; address: string; port: number; username: string; credentialId: string
  authKind: 'credential' | 'agent'; jumpHostId: string; groupId: string; tags: string; notes: string
}
const EMPTY: HostForm = {
  name: '', address: '', port: 22, username: 'root', credentialId: '',
  authKind: 'credential', jumpHostId: '', groupId: '', tags: '', notes: '',
}

export function HostsPage() {
  const toast = useToast()
  const [q, setQ] = useState('')
  const dq = useDebouncedValue(q)
  const [creating, setCreating] = useState(false)
  const [importing, setImporting] = useState(false)
  const [form, setForm] = useState(EMPTY)
  const hosts = useAsync(() => HostApi.list(dq ? { q: dq } : undefined), [dq], { intervalMs: 10_000 })
  const creds = useAsync(() => CredApi.list(), [])
  const groups = useAsync(() => HostApi.groups(), [])

  const credMap = useMemo(() => {
    const m = new Map<string, Credential>()
    ;(creds.data?.credentials ?? []).forEach((c) => m.set(c.id, c))
    return m
  }, [creds.data])

  async function create() {
    try {
      await HostApi.create({
        name: form.name || form.address,
        address: form.address,
        port: Number(form.port) || 22,
        username: form.username,
        credentialId: form.credentialId || undefined,
        authKind: form.authKind,
        jumpHostId: form.jumpHostId || undefined,
        groupId: form.groupId || undefined,
        tags: form.tags.split(',').map((s) => s.trim()).filter(Boolean),
        notes: form.notes,
      })
      toast.success('主机已添加')
      setCreating(false)
      setForm(EMPTY)
      hosts.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '添加失败') }
  }

  async function test(id: string) {
    try {
      const r = await HostApi.test(id)
      if (r.ok) toast.success(`连接成功，指纹 ${r.fingerprint}`)
      else toast.error(`探测失败：${r.status}`)
      hosts.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '探测失败') }
  }

  return (
    <>
      <div className="topbar"><h1>主机</h1></div>
      <div className="content">
        <ErrorBox message={hosts.error} />
        <PageTitle
          title={`主机列表（${hosts.data?.hosts.length ?? 0}）`}
          extra={
            <div className="toolbar" style={{ margin: 0 }}>
              <input placeholder="搜索名称/地址/标签" value={q} onChange={(e) => setQ(e.target.value)} style={{ width: 240 }} />
              <button onClick={() => setImporting(true)}>导入 SSH config</button>
              <button className="primary" onClick={() => setCreating(true)}>+ 添加主机</button>
            </div>
          }
        />
        {hosts.loading ? <Loading /> : (hosts.data?.hosts.length === 0) ? (
          <Empty text="还没有主机，点击右上角添加" />
        ) : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead>
                <tr>
                  <th>名称</th><th>地址</th><th>用户</th><th>凭据</th><th>标签</th>
                  <th>状态</th><th style={{ width: 230 }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {(hosts.data?.hosts ?? []).map((h: Host) => (
                  <tr key={h.id}>
                    <td><Link to={`/hosts/${h.id}`}>{h.name}</Link>{h.favorite && ' ★'}</td>
                    <td className="mono">{h.address}:{h.port}</td>
                    <td>{h.username}</td>
                    <td className="muted">{h.credentialId ? credMap.get(h.credentialId)?.name ?? shortId(h.credentialId) : '—'}</td>
                    <td className="muted">{(h.tags ?? []).join(', ')}</td>
                    <td><StatusBadge status={h.lastStatus || 'pending'} label={h.lastStatus || '未探测'} /></td>
                    <td>
                      <div className="row">
                        <button className="sm" onClick={() => test(h.id)}>探测</button>
                        <Link className="button" to={`/hosts/${h.id}`}><button className="sm">详情/终端</button></Link>
                        <ConfirmButton danger message={`删除主机 ${h.name}？`} onConfirm={async () => {
                          await HostApi.remove(h.id); hosts.reload(); toast.success('已删除')
                        }}>
                          <button className="sm danger">删除</button>
                        </ConfirmButton>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {creating && (
          <Modal title="添加主机" onClose={() => setCreating(false)}
            footer={<><button onClick={() => setCreating(false)}>取消</button>
              <button className="primary" onClick={create}>保存</button></>}>
            <div className="split">
              <label className="field"><span>名称</span>
                <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="可选，默认用地址" /></label>
              <label className="field"><span>地址</span>
                <input value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} placeholder="10.0.0.1" /></label>
              <label className="field"><span>端口</span>
                <input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: Number(e.target.value) })} /></label>
              <label className="field"><span>用户名</span>
                <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} /></label>
              <label className="field"><span>认证方式</span>
                <select value={form.authKind} onChange={(e) => setForm({ ...form, authKind: e.target.value as 'credential'|'agent' })}>
                  <option value="credential">凭据（密码/密钥）</option>
                  <option value="agent">bd-agent</option>
                </select></label>
              <label className="field"><span>凭据</span>
                <select value={form.credentialId} onChange={(e) => setForm({ ...form, credentialId: e.target.value })}>
                  <option value="">请选择</option>
                  {(creds.data?.credentials ?? []).map((c) => <option key={c.id} value={c.id}>{c.name}（{c.kind}）</option>)}
                </select></label>
              <label className="field"><span>跳板主机 ID（可选，最多 5 级，禁环）</span>
                <select value={form.jumpHostId} onChange={(e) => setForm({ ...form, jumpHostId: e.target.value })}>
                  <option value="">无</option>
                  {(hosts.data?.hosts ?? []).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
                </select></label>
              <label className="field"><span>分组</span>
                <select value={form.groupId} onChange={(e) => setForm({ ...form, groupId: e.target.value })}>
                  <option value="">无分组</option>
                  {(groups.data?.groups ?? []).map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
                </select></label>
            </div>
            <label className="field"><span>标签（逗号分隔）</span>
              <input value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} /></label>
            <label className="field"><span>备注</span>
              <textarea rows={2} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label>
          </Modal>
        )}

        <GroupsPanel
          groups={groups.data?.groups ?? []}
          hosts={hosts.data?.hosts ?? []}
          reload={() => { groups.reload(); hosts.reload() }}
        />

        {importing && <ImportModal onClose={() => setImporting(false)} onDone={() => { setImporting(false); hosts.reload() }}
          credentials={creds.data?.credentials ?? []} />}
      </div>
    </>
  )
}

function ImportModal({ onClose, onDone, credentials }: {
  onClose: () => void; onDone: () => void; credentials: Credential[]
}) {
  const toast = useToast()
  const [text, setText] = useState('')
  const [credentialId, setCredentialId] = useState('')
  const [defaultUser, setDefaultUser] = useState('root')
  const [result, setResult] = useState<unknown[] | null>(null)

  async function run() {
    try {
      const r = await HostApi.importSSHConfig(text, credentialId || undefined, defaultUser)
      setResult(r.imported)
      toast.success(`导入 ${r.count} 条`)
      onDone()
    } catch (e) { toast.error(e instanceof Error ? e.message : '导入失败') }
  }
  return (
    <Modal title="导入 ~/.ssh/config" onClose={onClose} width={680}
      footer={<><button onClick={onClose}>关闭</button><button className="primary" onClick={run}>解析并导入</button></>}>
      <label className="field"><span>默认用户</span>
        <input value={defaultUser} onChange={(e) => setDefaultUser(e.target.value)} /></label>
      <label className="field"><span>统一凭据</span>
        <select value={credentialId} onChange={(e) => setCredentialId(e.target.value)}>
          <option value="">不指定</option>
          {credentials.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select></label>
      <label className="field"><span>粘贴 ssh config 文本（通配符条目会被跳过）</span>
        <textarea rows={12} className="mono" value={text} onChange={(e) => setText(e.target.value)}
          placeholder={'Host web-1\n  HostName 10.0.0.11\n  User deploy\n  Port 22'} /></label>
      {result && <pre className="mono muted">{JSON.stringify(result, null, 2)}</pre>}
    </Modal>
  )
}
