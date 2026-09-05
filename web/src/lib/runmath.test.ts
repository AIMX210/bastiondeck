import { describe, expect, it } from 'vitest'
import {
  aggregateStatus, durationMs, emptySummary, fmtDuration, isLive, isTerminal,
  progressPct, rollupRuns, summarize,
} from './runmath'
import type { Run, RunTarget } from '@/api/types'

function t(id: string, status: string): RunTarget {
  return { id, runId: 'r', hostId: id, status, bytesOut: 0 }
}

describe('aggregateStatus mirrors server priority', () => {
  it('empty → pending', () => expect(aggregateStatus([])).toBe('pending'))
  it('all success → success', () => {
    expect(aggregateStatus([t('a', 'success'), t('b', 'success')])).toBe('success')
  })
  it('cancelled beats failed beats success', () => {
    expect(aggregateStatus([t('a', 'success'), t('b', 'failed'), t('c', 'cancelled')])).toBe('cancelled')
  })
  it('lost beats success', () => {
    expect(aggregateStatus([t('a', 'success'), t('b', 'lost')])).toBe('lost')
  })
  it('running beats pending beats success', () => {
    expect(aggregateStatus([t('a', 'success'), t('b', 'pending'), t('c', 'running')])).toBe('running')
  })
})

describe('summarize', () => {
  it('counts each state', () => {
    const s = summarize([t('1', 'success'), t('2', 'success'), t('3', 'failed'), t('4', 'lost')])
    expect(s.total).toBe(4)
    expect(s.success).toBe(2)
    expect(s.failed).toBe(1)
    expect(s.lost).toBe(1)
  })
  it('empty summary is zeroed', () => {
    const s = emptySummary()
    expect(s.total).toBe(0)
    expect(progressPct(s)).toBe(0)
  })
})

describe('terminal/live', () => {
  it('classifies states', () => {
    expect(isTerminal('success')).toBe(true)
    expect(isTerminal('running')).toBe(false)
    expect(isLive({ status: 'running' } as Run)).toBe(true)
    expect(isLive({ status: 'failed' } as Run)).toBe(false)
  })
})

describe('progress', () => {
  it('rounds completion', () => {
    const s = summarize([t('1', 'success'), t('2', 'running'), t('3', 'failed'), t('4', 'pending')])
    expect(progressPct(s)).toBe(50)
  })
})

describe('rollup', () => {
  it('sums summaries', () => {
    const runs = [
      { summary: summarize([t('a', 'success'), t('b', 'failed')]) },
      { summary: summarize([t('c', 'lost'), t('d', 'running')]) },
    ] as Run[]
    const r = rollupRuns(runs)
    expect(r.success).toBe(1)
    expect(r.failed).toBe(1)
    expect(r.lost).toBe(1)
    expect(r.running).toBe(1)
    expect(r.total).toBe(4)
  })
})

describe('duration', () => {
  it('computes delta', () => {
    expect(durationMs('2026-01-01T00:00:00Z', '2026-01-01T00:00:01Z')).toBe(1000)
    expect(durationMs('bad', '2026-01-01T00:00:01Z')).toBeNull()
    expect(durationMs('2026-01-01T00:00:02Z', '2026-01-01T00:00:01Z')).toBeNull()
  })
  it('formats', () => {
    expect(fmtDuration(null)).toBe('—')
    expect(fmtDuration(250)).toBe('250ms')
    expect(fmtDuration(2500)).toBe('2.5s')
    expect(fmtDuration(90_000)).toBe('1m30s')
  })
})
