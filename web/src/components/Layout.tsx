import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '@/store/auth'
import { classNames } from '@/lib/format'

// `perm` is the minimum permission required to even see the entry; pages that
// need more gate themselves. Omitting perm means every authenticated role sees it.
const NAV: { to: string; icon: string; label: string; end?: boolean; perm?: string }[] = [
  { to: '/', icon: '▦', label: '仪表盘', end: true },
  { to: '/hosts', icon: '🖧', label: '主机' },
  { to: '/files', icon: '🗀', label: '文件', perm: 'exec' },
  { to: '/exec', icon: '⚡', label: '批量执行', perm: 'exec' },
  { to: '/runs', icon: '▶', label: '任务与执行' },
  { to: '/jobs', icon: '⏲', label: '计划任务' },
  { to: '/tunnels', icon: '⇄', label: '隧道' },
  { to: '/snippets', icon: '✦', label: '命令片段' },
  { to: '/credentials', icon: '🔑', label: '凭据', perm: 'exec' },
  { to: '/agents', icon: '🛰', label: 'Agent', perm: 'manage_inventory' },
  { to: '/audit', icon: '⛓', label: '审计', perm: 'audit' },
  { to: '/settings', icon: '⚙', label: '设置' },
  { to: '/users', icon: '👥', label: '用户', perm: 'manage_users' },
  { to: '/backup', icon: '💾', label: '备份', perm: 'owner' },
]

export function Layout() {
  const { user, logout, can } = useAuth()
  const navigate = useNavigate()
  const items = NAV.filter((n) => !n.perm || can(n.perm))
  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">◆ BastionDeck</div>
        {items.map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            end={n.end}
            className={({ isActive }) => classNames('nav-item', isActive && 'active')}
          >
            <span className="nav-icon">{n.icon}</span>
            <span>{n.label}</span>
          </NavLink>
        ))}
        <div className="sidebar-foot">
          <div>{user?.displayName || user?.username}</div>
          <div className="muted" style={{ marginBottom: 8 }}>角色：{user?.role}</div>
          <button className="sm ghost" onClick={async () => { await logout(); navigate('/login') }}>
            退出登录
          </button>
        </div>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
  )
}
