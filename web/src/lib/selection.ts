import type { Host } from '@/api/types'

export type SelectionMode = 'all' | 'pick' | 'group' | 'tag'

export interface SelectionSpec {
  mode: SelectionMode
  picked: Set<string>
  groupId: string
  tag: string
}

// Pure target selection used by the batch execution wizard. Keeping it pure
// makes the "which machines will I touch" question unit-testable.
export function selectHosts(all: Host[], spec: SelectionSpec): Host[] {
  switch (spec.mode) {
    case 'all':
      return all.slice()
    case 'pick':
      return all.filter((h) => spec.picked.has(h.id))
    case 'group':
      return spec.groupId ? all.filter((h) => h.groupId === spec.groupId) : []
    case 'tag':
      return spec.tag ? all.filter((h) => (h.tags ?? []).includes(spec.tag)) : []
  }
}

export function collectTags(all: Host[]): string[] {
  const s = new Set<string>()
  all.forEach((h) => (h.tags ?? []).forEach((t) => s.add(t)))
  return Array.from(s).sort()
}

export function collectGroupIds(all: Host[]): string[] {
  const s = new Set<string>()
  all.forEach((h) => { if (h.groupId) s.add(h.groupId) })
  return Array.from(s).sort()
}

// Extract ${var} placeholders from a command body, sorted and de-duplicated.
export function requiredVariables(body: string): string[] {
  const re = /\$\{\s*([a-zA-Z0-9_.-]+)\s*\}/g
  const found = new Set<string>()
  let m: RegExpExecArray | null
  while ((m = re.exec(body)) !== null) found.add(m[1])
  return Array.from(found).sort()
}

// Fill a template; returns missing variable names so callers can block
// execution instead of sending a half-rendered command.
export function renderTemplate(body: string, vars: Record<string, string>): {
  rendered: string
  missing: string[]
} {
  const missing: string[] = []
  const rendered = body.replace(/\$\{\s*([a-zA-Z0-9_.-]+)\s*\}/g, (whole, name: string) => {
    if (Object.prototype.hasOwnProperty.call(vars, name) && vars[name] !== '') return vars[name]
    if (!missing.includes(name)) missing.push(name)
    return whole
  })
  return { rendered, missing }
}

// Stable de-duplication of host ids while preserving first-seen order.
export function dedupeIds(ids: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const id of ids) {
    if (!seen.has(id)) { seen.add(id); out.push(id) }
  }
  return out
}
