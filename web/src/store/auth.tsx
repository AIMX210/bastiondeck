import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { AuthApi, StatusApi } from '@/api/endpoints'
import { setToken } from '@/api/client'
import type { User } from '@/api/types'

interface AuthState {
  user: User | null
  permissions: string[]
  setupRequired: boolean | null
  loading: boolean
  login: (username: string, password: string, totp?: string) => Promise<void>
  setup: (username: string, password: string, displayName?: string) => Promise<void>
  logout: () => Promise<void>
  refresh: () => Promise<void>
  can: (perm: string) => boolean
}

const Ctx = createContext<AuthState | null>(null)
const ROLE_ORDER: Record<string, number> = { viewer: 0, operator: 1, admin: 2, owner: 3 }

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [permissions, setPermissions] = useState<string[]>([])
  const [setupRequired, setSetupRequired] = useState<boolean | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    const st = await StatusApi.status()
    setSetupRequired(st.setupRequired)
    if (st.setupRequired) { setUser(null); return }
    try {
      const me = await AuthApi.me()
      setUser(me.user)
      setPermissions(me.permissions)
    } catch {
      setUser(null)
      setPermissions([])
    }
  }, [])

  useEffect(() => {
    refresh().finally(() => setLoading(false))
  }, [refresh])

  const login = useCallback(async (username: string, password: string, totp?: string) => {
    const r = await AuthApi.login({ username, password, totp })
    setToken(r.token)
    await refresh()
  }, [refresh])

  const setup = useCallback(async (username: string, password: string, displayName?: string) => {
    await AuthApi.setup({ username, password, displayName })
    await login(username, password)
  }, [login])

  const logout = useCallback(async () => {
    await AuthApi.logout().catch(() => undefined)
    setToken(null)
    setUser(null)
    setPermissions([])
    await refresh()
  }, [refresh])

  const can = useCallback((perm: string) => permissions.includes(perm), [permissions])

  const value = useMemo<AuthState>(() => ({
    user, permissions, setupRequired, loading, login, setup, logout, refresh, can,
  }), [user, permissions, setupRequired, loading, login, setup, logout, refresh, can])

  // Expose role ordering for guards.
  ;(value as AuthState & { roleAtLeast?: (r: string) => boolean }).roleAtLeast = (r: string) =>
    ! !user && ROLE_ORDER[user.role] >= ROLE_ORDER[r]

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useAuth(): AuthState {
  const v = useContext(Ctx)
  if (!v) throw new Error('useAuth must be used within AuthProvider')
  return v
}
