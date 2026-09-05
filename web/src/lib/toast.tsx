import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'

type ToastKind = 'info' | 'success' | 'error'
interface Toast { id: number; kind: ToastKind; text: string }

interface ToastCtx {
  push: (kind: ToastKind, text: string) => void
  info: (t: string) => void
  success: (t: string) => void
  error: (t: string) => void
}

const Ctx = createContext<ToastCtx | null>(null)
let seq = 0

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const remove = useCallback((id: number) => {
    setToasts((ts) => ts.filter((t) => t.id !== id))
  }, [])
  const push = useCallback((kind: ToastKind, text: string) => {
    const id = ++seq
    setToasts((ts) => [...ts, { id, kind, text }])
    window.setTimeout(() => remove(id), kind === 'error' ? 6000 : 3000)
  }, [remove])
  const value = useMemo<ToastCtx>(() => ({
    push,
    info: (t) => push('info', t),
    success: (t) => push('success', t),
    error: (t) => push('error', t),
  }), [push])
  return (
    <Ctx.Provider value={value}>
      {children}
      <div className="toast-stack">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.kind}`} onClick={() => remove(t.id)}>
            {t.text}
          </div>
        ))}
      </div>
    </Ctx.Provider>
  )
}

export function useToast(): ToastCtx {
  const v = useContext(Ctx)
  if (!v) throw new Error('useToast must be used within ToastProvider')
  return v
}
