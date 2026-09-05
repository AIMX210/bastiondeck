import { useState } from 'react'
import { CredApi } from '@/api/endpoints'
import type { Credential } from '@/api/types'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { Empty, ErrorBox, Loading, PageTitle } from '@/components/Common'
import { Modal } from '@/components/Modal'
import { ConfirmButton } from '@/components/Confirm'
import { fmtTime, shortId } from '@/lib/format'

export function CredentialsPage() {
  const toast = useToast()
  const list = useAsync(() => CredApi.list(), [])
  const [form, setForm] = useState<{ name: string; kind: 'password'|'private_key'; secret: string; passphrase: string } | null>(null)

  async function save() {
    if (!form) return
    try {
      await CredApi.create({
        name: form.name, kind: form.kind, secret: form.secret,
        passphrase: form.kind === 'private_key' ? form.passphrase || undefined : undefined,
      })
      toast.success('凭据已加密入库（AES-256-GCM，仅连接时解密）')
      setForm(null); list.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '保存失败') }
  }

  return (
    <>
      <div className="topbar"><h1>凭据保险库</h1></div>
      <div className="content">
        <ErrorBox message={list.error} />
        <PageTitle title="凭据（密文存储，界面永不回显明文）" extra={
          <button className="primary" onClick={() => setForm({ name: '', kind: 'password', secret: '', passphrase: '' })}>+ 添加凭据</button>
        } />
        {list.loading ? <Loading /> : (list.data?.credentials.length === 0) ? <Empty text="暂无凭据" /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead><tr><th>名称</th><th>类型</th><th>指纹</th><th>创建时间</th><th></th></tr></thead>
              <tbody>
                {(list.data?.credentials ?? []).map((c: Credential) => (
                  <tr key={c.id}>
                    <td>{c.name}</td>
                    <td>{c.kind === 'password' ? '密码' : '私钥'}</td>
                    <td className="mono muted">{c.fingerprint ? shortId(c.fingerprint, 20) : '—'}</td>
                    <td className="muted">{fmtTime(c.createdAt)}</td>
                    <td>
                      <ConfirmButton danger message="删除该凭据？引用它的主机将无法连接" onConfirm={async () => {
                        await CredApi.remove(c.id); list.reload()
                      }}><button className="sm danger">删除</button></ConfirmButton>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {form && (
          <Modal title="添加凭据" onClose={() => setForm(null)}
            footer={<><button onClick={() => setForm(null)}>取消</button>
              <button className="primary" onClick={save}>加密保存</button></>}>
            <label className="field"><span>名称</span>
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
            <label className="field"><span>类型</span>
              <select value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value as 'password'|'private_key' })}>
                <option value="password">密码</option>
                <option value="private_key">SSH 私钥（PEM/OpenSSH）</option>
              </select></label>
            <label className="field">
              <span>{form.kind === 'password' ? '密码' : '私钥内容（-----BEGIN ...-----）'}</span>
              <textarea rows={form.kind === 'private_key' ? 8 : 2}
                value={form.secret} onChange={(e) => setForm({ ...form, secret: e.target.value })} />
            </label>
            {form.kind === 'private_key' && (
              <label className="field"><span>私钥口令（可选）</span>
                <input type="password" value={form.passphrase} onChange={(e) => setForm({ ...form, passphrase: e.target.value })} /></label>
            )}
          </Modal>
        )}
      </div>
    </>
  )
}
