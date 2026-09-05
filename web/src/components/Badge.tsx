import { classNames } from '@/lib/format'
import { statusLabel } from '@/lib/format'

const CLASS: Record<string, string> = {
  success: 'b-success', failed: 'b-failed', lost: 'b-lost',
  running: 'b-running', pending: 'b-pending', timeout: 'b-timeout',
  cancelled: 'b-cancelled', skipped: 'b-skipped',
  online: 'b-success', offline: 'b-timeout', approved: 'b-success',
  blocked: 'b-failed',
}

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const cls = CLASS[status] ?? 'b-timeout'
  return (
    <span className={classNames('badge', cls)}>
      <span className="dot" />
      {label ?? statusLabel(status)}
    </span>
  )
}

export function Badge({ children, tone }: { children: React.ReactNode; tone?: string }) {
  return <span className={classNames('badge', tone && `b-${tone}`)}>{children}</span>
}
