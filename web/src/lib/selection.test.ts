import { describe, expect, it } from 'vitest'
import {
  collectGroupIds, collectTags, dedupeIds, renderTemplate, requiredVariables, selectHosts,
} from './selection'
import type { Host } from '@/api/types'

function host(id: string, extra: Partial<Host> = {}): Host {
  return {
    id, name: id, address: id, port: 22, username: 'root', authKind: 'credential',
    tags: [], notes: '', favorite: false, lastStatus: 'pending', createdAt: '', updatedAt: '',
    ...extra,
  } as Host
}

const FLEET = [
  host('a', { groupId: 'g1', tags: ['web', 'prod'] }),
  host('b', { groupId: 'g1', tags: ['web'] }),
  host('c', { groupId: 'g2', tags: ['db', 'prod'] }),
]

describe('selectHosts', () => {
  it('all returns every host', () => {
    expect(selectHosts(FLEET, { mode: 'all', picked: new Set(), groupId: '', tag: '' })).toHaveLength(3)
  })
  it('pick honors the explicit set', () => {
    const got = selectHosts(FLEET, { mode: 'pick', picked: new Set(['a', 'c']), groupId: '', tag: '' })
    expect(got.map((h) => h.id)).toEqual(['a', 'c'])
  })
  it('group filters by groupId and empty group yields none', () => {
    expect(selectHosts(FLEET, { mode: 'group', picked: new Set(), groupId: 'g1', tag: '' }).map((h) => h.id)).toEqual(['a', 'b'])
    expect(selectHosts(FLEET, { mode: 'group', picked: new Set(), groupId: '', tag: '' })).toEqual([])
  })
  it('tag filters membership', () => {
    expect(selectHosts(FLEET, { mode: 'tag', picked: new Set(), groupId: '', tag: 'prod' }).map((h) => h.id)).toEqual(['a', 'c'])
  })
})

describe('collect helpers', () => {
  it('collectTags sorted unique', () => {
    expect(collectTags(FLEET)).toEqual(['db', 'prod', 'web'])
  })
  it('collectGroupIds sorted unique non-empty', () => {
    expect(collectGroupIds([...FLEET, host('d')])).toEqual(['g1', 'g2'])
  })
})

describe('template variables', () => {
  it('requiredVariables sorted unique', () => {
    expect(requiredVariables('${z} ${a} ${z}')).toEqual(['a', 'z'])
  })
  it('renderTemplate fills and reports missing', () => {
    const ok = renderTemplate('hi ${name}', { name: 'ops' })
    expect(ok.rendered).toBe('hi ops')
    expect(ok.missing).toEqual([])
    const bad = renderTemplate('${a}-${b}', { a: '1' })
    expect(bad.rendered).toBe('1-${b}')
    expect(bad.missing).toEqual(['b'])
    const empty = renderTemplate('${a}', { a: '' })
    expect(empty.missing).toEqual(['a'])
  })
  it('dedupeIds keeps first-seen order', () => {
    expect(dedupeIds(['x', 'y', 'x', 'z', 'y'])).toEqual(['x', 'y', 'z'])
  })
})
