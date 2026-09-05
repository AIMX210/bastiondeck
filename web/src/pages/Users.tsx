import { useState } from 'react'
import { AuthApi, UsersApi } from '@/api/endpoints'
import type { User } from '@/api/types'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { useAuth } from '@/store/auth'
import { ErrorBox, Loading, PageTitle } from '@/components/Common'
import { Modal } from '@/components/Modal'
import { ConfirmButton } from '@/components/Confirm'
import { fmtTime } from '@/lib/format'

const ROLES = ['viewer', 'operator', 'admin', 'owner']

export function UsersPage() {
  const toast = useToast()
  const { user: me } = useAuth()
  const list = useAsync(() => UsersApi.list(), [])
  const sessions = useAsync(() => UsersApi.mySessions(), [])
  const [form, setForm] = useState<{ username: string; displayName: string; role: string; password: string } | null>(null)
  const [pw, setPw] = useState<{ oldPassword: string; newPassword: string } | null>(null)
  const [totp, setTotp] = useState<{ secret: string; uri: string; code: string } | null>(null)

  async function createUser() {
    if (!form) return
    try {
      await UsersApi.create(form)
      toast.success('用户已创建'); setForm(null); list.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '创建失败') }
  }

  async function startTotp() {
    const r = await AuthApi.totpSetup()
    setTotp({ ...r, code: '' })
  }

  const isOwner = me?.role === 'owner'

  return (
    <>
      <div className="topbar"><h1>用户与会话</h1></div>
      <div className="content">
        <ErrorBox message={list.error} />
        <PageTitle title="用户（viewer &lt; operator &lt; admin &lt; owner）" extra={
          isOwner ? <button className="primary" onClick={() => setForm({
            username: '', displayName: '', role: 'operator', password: '',
          })}>+ 新建用户</button> : <span className="muted">仅 owner 可管理用户</span>
        } />
        {list.loading ? <Loading /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead><tr><th>用户名</th><th>显示名</th><th>角色</th><th>TOTP</th><th>最近登录</th><th style={{ width: 160 }}></th></tr></thead>
              <tbody>
                {(list.data?.users ?? []).map((u: User) => (
                  <tr key={u.id}>
                    <td>{u.username}{u.id === me?.id && '（我）'}</td>
                    <td>{u.displayName}</td>
                    <td>
                      {isOwner && u.id !== me?.id ? (
                        <select value={u.role} onChange={async (e) => {
                          await UsersApi.update(u.id, { role: e.target.value }); list.reload()
                        }}>
                          {ROLES.map((r) => <option key={r}>{r}</option>)}
                        </select>
                      ) : u.role}
                    </td>
                    <td>{u.totpEnabled ? '已启用' : '未启用'}</td>
                    <td className="muted">{fmtTime(u.lastLoginAt)}</td>
                    <td>
                      {isOwner && u.id !== me?.id && (
                        <ConfirmButton danger message={`删除用户 ${u.username}？`} onConfirm={async () => {
                          await UsersApi.remove(u.id); list.reload()
                        }}><button className="sm danger">删除</button></ConfirmButton>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div className="panel">
          <div className="toolbar" style={{ marginBottom: 10 }}>
            <h2 style={{ margin: 0 }}>我的会话（滑动过期，服务端只存摘要）</h2>
            <div className="grow" />
            <button onClick={() => setPw({ oldPassword: '', newPassword: '' })}>修改密码</button>
            <button onClick={startTotp}>绑定 TOTP</button>
            <ConfirmButton danger message="吊销除当前会话外的全部会话？" onConfirm={async () => {
              await UsersApi.revokeAll(); sessions.reload(); toast.info('已吊销')
            }}><button className="sm danger">吊销其他会话</button></ConfirmButton>
          </div>
          <table className="grid">
            <thead><tr><th>创建</th><th>过期</th><th>最近活跃</th><th>IP</th><th>UA</th><th></th></tr></thead>
            <tbody>
              {(sessions.data?.sessions ?? []).map((s) => (
                <tr key={s.id}>
                  <td className="muted">{fmtTime(s.createdAt)}</td>
                  <td className="muted">{fmtTime(s.expiresAt)}</td>
                  <td className="muted">{fmtTime(s.lastSeenAt)}</td>
                  <td className="mono">{s.ip}</td>
                  <td className="muted" style={{ maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.userAgent}</td>
                  <td>{!s.current && <button className="sm danger" onClick={async () => {
                    await UsersApi.revoke(s.id); sessions.reload()
                  }}>吊销</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {form && (
          <Modal title="新建用户" onClose={() => setForm(null)}
            footer={<><button onClick={() => setForm(null)}>取消</button>
              <button className="primary" onClick={createUser}>创建</button></>}>
            <label className="field"><span>用户名</span>
              <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} /></label>
            <label className="field"><span>显示名</span>
              <input value={form.displayName} onChange={(e) => setForm({ ...form, displayName: e.target.value })} /></label>
            <label className="field"><span>角色</span>
              <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                {ROLES.map((r) => <option key={r}>{r}</option>)}
              </select></label>
            <label className="field"><span>初始密码（≥10 位，首次登录建议修改）</span>
              <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} /></label>
          </Modal>
        )}

        {pw && (
          <Modal title="修改密码" onClose={() => setPw(null)}
            footer={<><button onClick={() => setPw(null)}>取消</button>
              <button className="primary" onClick={async () => {
                await AuthApi.changePassword(pw.oldPassword, pw.newPassword)
                toast.success('密码已更新'); setPw(null)
              }}>保存</button></>}>
            <label className="field"><span>当前密码</span>
              <input type="password" value={pw.oldPassword} onChange={(e) => setPw({ ...pw, oldPassword: e.target.value })} /></label>
            <label className="field"><span>新密码（≥10 位）</span>
              <input type="password" value={pw.newPassword} onChange={(e) => setPw({ ...pw, newPassword: e.target.value })} /></label>
          </Modal>
        )}

        {totp && (
          <Modal title="绑定 TOTP（RFC 6238）" onClose={() => setTotp(null)}
            footer={<button className="primary" onClick={async () => {
              await AuthApi.totpEnable(totp.code); toast.success('TOTP 已启用'); setTotp(null); list.reload()
            }}>确认绑定</button>}>
            <p className="muted">把以下密钥录入验证器（或扫描 otpauth 二维码）：</p>
            <pre className="mono panel">{totp.secret}{'\n'}{totp.uri}</pre>
            <label className="field"><span>输入当前 6 位验证码</span>
              <input value={totp.code} onChange={(e) => setTotp({ ...totp, code: e.target.value })} maxLength={6} /></label>
          </Modal>
        )}
      </div>
    </>
  )
}
