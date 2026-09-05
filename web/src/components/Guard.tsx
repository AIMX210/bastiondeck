import { Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from '@/store/auth'
import { Loading } from './Common'

// Gate the whole app: bootstrap → setup page; unauthenticated → login.
export function Gate({ children }: { children: ReactNode }) {
  const { user, setupRequired, loading } = useAuth()
  const loc = useLocation()
  if (loading || setupRequired === null) return <div className="auth-wrap"><Loading label="正在连接…" /></div>
  if (setupRequired) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname }} />
  return <>{children}</>
}

// Per-permission guard for sensitive pages.
export function RequirePerm({ perm, children }: { perm: string; children: ReactNode }) {
  const { can } = useAuth()
  if (!can(perm)) {
    return <div className="content"><div className="empty">缺少权限：{perm}</div></div>
  }
  return <>{children}</>
}
