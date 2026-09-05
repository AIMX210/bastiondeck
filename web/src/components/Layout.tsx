import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '@/store/auth'
import { classNames } from '@/lib/format'

const NAV = [
  { to: '/', icon: '▦', label: '仪表盘', end: true },
  { to: '/hosts', icon: '🖧', label: '主机' },
  { to: '/files', icon: '🗀', label: '文件' },
  { to: '/exec', icon: '⚡', label: '批量执行' },
  { to: '/runs', icon: '▶', label: '任务与执行' },
  { to: '/jobs', icon: '⏲', label: '计划任务' },
  { to: '/tunnels', icon: '⇄', label: '隧道' },
  { to: '/snippets', icon: '✦', label: '命令片段' },
  { to: '/credentials', icon: '🔑', label: '凭据' },
  { to: '/agents', icon: '🛰', label: 'Agent' },
  { to: '/audit', icon: '⛓', label: '审计' },
  { to: '/settings', icon: '⚙', label: '设置' },
  { to: '/users', icon: '👥', label: '用户' },
  { to: '/backup', icon: '💾', label: '备份' },
]

export function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">◆ BastionDeck</div>
        {NAV.map((n) => (
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
