import type { Run, RunSummary, RunTarget } from '@/api/types'

// Aggregation priority mirrors jobs/statemachine.go on the server: a run is
// only "success" when every target succeeded; the most severe target state
// wins. Keeping the same ordering client-side makes the UI agree with the
// server even before the authoritative summary arrives.
const PRIORITY: Record<string, number> = {
  cancelled: 6,
  timeout: 5,
  failed: 4,
  lost: 3,
  running: 2,
  pending: 1,
  skipped: 0,
  success: -1,
}

export function aggregateStatus(targets: RunTarget[]): string {
  if (targets.length === 0) return 'pending'
  let worst = 'success'
  for (const t of targets) {
    if ((PRIORITY[t.status] ?? PRIORITY.failed) > (PRIORITY[worst] ?? 0)) {
      worst = t.status
    }
  }
  return worst
}

export function emptySummary(): RunSummary {
  return {
    total: 0, pending: 0, running: 0, success: 0, failed: 0,
    timeout: 0, cancelled: 0, lost: 0, skipped: 0,
  }
}

export function summarize(targets: RunTarget[]): RunSummary {
  const s = emptySummary()
  s.total = targets.length
  const acc = s as unknown as Record<string, number>
  for (const t of targets) {
    if (t.status in acc) acc[t.status] += 1
  }
  return s
}

export const TERMINAL = new Set(['success', 'failed', 'timeout', 'cancelled', 'lost', 'skipped'])

export function isTerminal(status: string): boolean {
  return TERMINAL.has(status)
}

export function isLive(run: Pick<Run, 'status'>): boolean {
  return run.status === 'pending' || run.status === 'running'
}

// Progress percentage for progress bars, rounded to an integer.
export function progressPct(summary: RunSummary): number {
  if (summary.total === 0) return 0
  const done = summary.success + summary.failed + summary.timeout +
    summary.cancelled + summary.lost + summary.skipped
  return Math.round((done / summary.total) * 100)
}

// Roll up recent runs into dashboard counters.
export function rollupRuns(runs: Run[]) {
  const acc = { success: 0, failed: 0, lost: 0, timeout: 0, running: 0, total: 0 }
  for (const r of runs) {
    acc.success += r.summary.success
    acc.failed += r.summary.failed
    acc.lost += r.summary.lost
    acc.timeout += r.summary.timeout
    acc.running += r.summary.running + r.summary.pending
    acc.total += r.summary.total
  }
  return acc
}

// Human-readable duration between two ISO timestamps (ms precision tolerated).
export function durationMs(start?: string, end?: string): number | null {
  if (!start || !end) return null
  const a = new Date(start).getTime()
  const b = new Date(end).getTime()
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) return null
  return b - a
}

export function fmtDuration(ms: number | null): string {
  if (ms === null) return '—'
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m${Math.round(s % 60)}s`
}
