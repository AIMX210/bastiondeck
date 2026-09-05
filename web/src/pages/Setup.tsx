import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/store/auth'
import { useToast } from '@/lib/toast'

// First-run owner bootstrap. The server refuses setup once an owner exists
// (setupRequired guard), preventing account-takeover by late visitors.
export function SetupPage() {
  const { setup } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [username, setUsername] = useState('owner')
  const [displayName, setDisplayName] = useState('管理员')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (password.length < 10) { toast.error('密码至少 10 位'); return }
    if (password !== confirm) { toast.error('两次输入的密码不一致'); return }
    setBusy(true)
    try {
      await setup(username, password, displayName)
      toast.success('初始化完成，已自动登录')
      navigate('/')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '初始化失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth-wrap">
      <form className="panel auth-card" onSubmit={submit}>
        <div className="brand">◆ 初始化 BastionDeck</div>
        <div className="auth-sub">
          首次使用：创建 Owner 账户。凭据保险库主密钥已在服务端生成，仅保存在本机数据目录。
        </div>
        <label className="field">
          <span>用户名</span>
          <input value={username} onChange={(e) => setUsername(e.target.value)} required />
        </label>
        <label className="field">
          <span>显示名</span>
          <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        </label>
        <label className="field">
          <span>密码（至少 10 位）</span>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        <label className="field">
          <span>确认密码</span>
          <input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
        </label>
        <button className="primary" style={{ width: '100%' }} disabled={busy}>
          {busy ? '创建中…' : '创建 Owner 并进入'}
        </button>
      </form>
    </div>
  )
}
