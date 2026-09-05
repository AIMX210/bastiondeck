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
const RANK: Record<string, number> = { viewer: 0, operator: 1, admin: 2, owner: 3 }

export function UsersPage() {
  const toast = useToast()
  const { user: me, can } = useAuth()
  const list = useAsync(() => UsersApi.list(), [])
  const sessions = useAsync(() => UsersApi.mySessions(), [])
  const [form, setForm] = useState<{ username: string; displayName: string; role: string; password: string } | null>(null)
  const [pw, setPw] = useState<{ oldPassword: string; newPassword: string } | null>(null)
  const [totp, setTotp] = useState<{ secret: string; uri: string; code: string } | null>(null)

  async function createUser() {
    if (!form) return
    if (!canAssign(form.role)) { toast.error('当前角色无权授予该角色'); return }
    if (form.password.length < 10) { toast.error('初始密码至少 10 位'); return }
    try {
      await UsersApi.create(form)
      toast.success('用户已创建'); setForm(null); list.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '创建失败') }
  }

  async function startTotp() {
    try {
      const r = await AuthApi.totpSetup()
      setTotp({ ...r, code: '' })
    } catch (e) { toast.error(e instanceof Error ? e.message : 'TOTP 初始化失败') }
  }

  // Mirror server-side CanAssignRole: owner grants anyone; admin only manages
  // strictly lower roles (viewer/operator) and never admin/owner.
  const canAssign = (targetRole: string): boolean => {
    if (!me) return false
    if (me.role === 'owner') return true
    if (targetRole === 'owner' || targetRole === 'admin') return false
    return (RANK[me.role] ?? 0) > (RANK[targetRole] ?? 99)
  }
  const canManage = can('manage_users')

  return (
    <>
      <div className="topbar"><h1>用户与会话</h1></div>
      <div className="content">
        <ErrorBox message={list.error} />
        <PageTitle title="用户（viewer &lt; operator &lt; admin &lt; owner）" extra={
          canManage ? <button className="primary" onClick={() => setForm({
            username: '', displayName: '', role: me?.role === 'owner' ? 'admin' : 'operator', password: '',
          })}>+ 新建用户</button> : <span className="muted">当前角色无权管理用户</span>
        } />
        {list.loading ? <Loading /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead><tr><th>用户名</th><th>显示名</th><th>角色</th><th>TOTP</th><th>最近登录</th><th style={{ width: 160 }}></th></tr></thead>
              <tbody>
                {(list.data?.users ?? []).map((u: User) => {
                  const editable = u.id !== me?.id && canAssign(u.role)
                  return (
                  <tr key={u.id}>
                    <td>{u.username}{u.id === me?.id && '（我）'}</td>
                    <td>{u.displayName}</td>
                    <td>
                      {editable ? (
                        <select value={u.role} onChange={async (e) => {
                          try { await UsersApi.update(u.id, { role: e.target.value }); list.reload() }
                          catch (err) { toast.error(err instanceof Error ? err.message : '更新失败') }
                        }}>
                          {ROLES.filter(canAssign).map((r) => <option key={r}>{r}</option>)}
                        </select>
                      ) : u.role}
                    </td>
                    <td>{u.totpEnabled ? '已启用' : '未启用'}</td>
                    <td className="muted">{fmtTime(u.lastLoginAt)}</td>
                    <td>
                      {editable && (
                        <ConfirmButton danger message={`删除用户 ${u.username}？`} onConfirm={async () => {
                          await UsersApi.remove(u.id); list.reload()
                        }}><button className="sm danger">删除</button></ConfirmButton>
                      )}
                    </td>
                  </tr>
                  )
                })}
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
                    try { await UsersApi.revoke(s.id); sessions.reload() }
                    catch (e) { toast.error(e instanceof Error ? e.message : '吊销失败') }
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
                {ROLES.filter(canAssign).map((r) => <option key={r}>{r}</option>)}
              </select></label>
            <label className="field"><span>初始密码（≥10 位，首次登录建议修改）</span>
              <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} /></label>
          </Modal>
        )}

        {pw && (
          <Modal title="修改密码" onClose={() => setPw(null)}
            footer={<><button onClick={() => setPw(null)}>取消</button>
              <button className="primary" onClick={async () => {
                if (pw.newPassword.length < 10) { toast.error('新密码至少 10 位'); return }
                try {
                  await AuthApi.changePassword(pw.oldPassword, pw.newPassword)
                  toast.success('密码已更新'); setPw(null)
                } catch (e) { toast.error(e instanceof Error ? e.message : '更新失败') }
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
              try {
                await AuthApi.totpEnable(totp.code); toast.success('TOTP 已启用'); setTotp(null); list.reload()
              } catch (e) { toast.error(e instanceof Error ? e.message : '绑定失败') }
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
