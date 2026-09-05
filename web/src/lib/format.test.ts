import { describe, expect, it } from 'vitest'
import { fmtBytes, modeString, shortId, statusLabel, classNames } from './format'

describe('format helpers', () => {
  it('shortId keeps short strings and truncates long ones', () => {
    expect(shortId('run_abc')).toBe('run_abc')
    expect(shortId('run_0123456789ABCDEF', 10)).toBe('run_012345')
    expect(shortId(undefined)).toBe('—')
  })

  it('fmtBytes uses binary units', () => {
    expect(fmtBytes(0)).toBe('0 B')
    expect(fmtBytes(1023)).toBe('1023 B')
    expect(fmtBytes(1024)).toBe('1.0 KiB')
    expect(fmtBytes(1024 * 1024 * 3)).toBe('3.0 MiB')
  })

  it('modeString renders rwx triplets', () => {
    expect(modeString(0o755)).toBe('rwxr-xr-x')
    expect(modeString(0o644)).toBe('rw-r--r--')
    expect(modeString(0o000)).toBe('---------')
  })

  it('statusLabel maps machine states to Chinese', () => {
    expect(statusLabel('success')).toBe('成功')
    expect(statusLabel('lost')).toBe('失联')
    expect(statusLabel('weird')).toBe('weird')
  })

  it('statusLabel covers tunnel / agent / audit states', () => {
    expect(statusLabel('active')).toBe('运行中')
    expect(statusLabel('starting')).toBe('建立中')
    expect(statusLabel('stopped')).toBe('已停止')
    expect(statusLabel('approved')).toBe('已批准')
    expect(statusLabel('blocked')).toBe('已拉黑')
    expect(statusLabel('denied')).toBe('已拒绝')
    expect(statusLabel('failure')).toBe('失败')
  })

  it('classNames drops falsy entries', () => {
    expect(classNames('a', false, null, undefined, 'b')).toBe('a b')
  })
})
