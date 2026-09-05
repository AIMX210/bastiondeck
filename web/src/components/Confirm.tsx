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
  return (
    <>
      <span onClick={() => setOpen(true)}>{children}</span>
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
                  try { await onConfirm(); setOpen(false) } finally { setBusy(false) }
                }}
              >
                {confirmText}
              </button>
            </>
          }
        >
          <p>{message}</p>
        </Modal>
      )}
    </>
  )
}
