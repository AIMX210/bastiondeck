import { useState, type FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '@/store/auth'
import { useToast } from '@/lib/toast'
import { ApiError } from '@/api/client'

export function LoginPage() {
  const { login } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const loc = useLocation()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [totp, setTotp] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      await login(username, password, totp || undefined)
      const from = (loc.state as { from?: string } | null)?.from
      navigate(from && from.startsWith('/') ? from : '/', { replace: true })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '登录失败'
      toast.error(msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth-wrap">
      <form className="panel auth-card" onSubmit={submit}>
        <div className="brand">◆ BastionDeck</div>
        <div className="auth-sub">自托管主机舰队管控台 · 请登录</div>
        <label className="field">
          <span>用户名</span>
          <input autoFocus value={username} onChange={(e) => setUsername(e.target.value)} required />
        </label>
        <label className="field">
          <span>密码</span>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        <label className="field">
          <span>动态口令（如已启用 TOTP）</span>
          <input value={totp} onChange={(e) => setTotp(e.target.value)} inputMode="numeric" maxLength={6} placeholder="6 位数字" />
        </label>
        <button className="primary" style={{ width: '100%' }} disabled={busy}>
          {busy ? '登录中…' : '登录'}
        </button>
      </form>
    </div>
  )
}
