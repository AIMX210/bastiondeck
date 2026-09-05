import { useState } from 'react'
import { Modal } from './Modal'

export function ConfirmButton({
  title = '确认操作', message, confirmText = '确认', danger, onConfirm, children,
}: {
  title?: string
  message: string
  confirmText?: string
  danger?: boolean
  onConfirm: () => Promise<void> | void
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  return (
    <>
      <span onClick={() => { setErr(null); setOpen(true) }}>{children}</span>
      {open && (
        <Modal
          title={title}
          onClose={() => setOpen(false)}
          footer={
            <>
              <button onClick={() => setOpen(false)}>取消</button>
              <button
                className={danger ? 'danger' : 'primary'}
                disabled={busy}
                onClick={async () => {
                  setBusy(true)
                  setErr(null)
                  try {
                    await onConfirm()
                    setOpen(false)
                  } catch (e) {
                    // Surface failures instead of silently leaving the user with no feedback.
                    setErr(e instanceof Error ? e.message : String(e))
                  } finally {
                    setBusy(false)
                  }
                }}
              >
                {busy ? '处理中…' : confirmText}
              </button>
            </>
          }
        >
          <p>{message}</p>
          {err && <div className="err-text" style={{ marginTop: 8 }}>操作失败：{err}</div>}
        </Modal>
      )}
    </>
  )
}
