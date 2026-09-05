// Wire DTOs mirroring the Go JSON contracts (docs/03-api-contract.md).

export interface User {
  id: string
  username: string
  displayName: string
  role: 'owner' | 'admin' | 'operator' | 'viewer'
  totpEnabled: boolean
  disabled: boolean
  mustChangePassword: boolean
  lastLoginAt?: string
  createdAt: string
  updatedAt: string
}

export interface Credential {
  id: string
  name: string
  kind: 'password' | 'private_key'
  fingerprint?: string
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface Host {
  id: string
  name: string
  address: string
  port: number
  username: string
  credentialId?: string
  authKind: 'credential' | 'agent'
  agentId?: string
  jumpHostId?: string
  groupId?: string
  tags: string[]
  notes: string
  favorite: boolean
  lastStatus: string
  knownHostKey?: string
  knownHostKeyType?: string
  firstSeenAt?: string
  lastConnectedAt?: string
  options?: Record<string, string>
  createdAt: string
  updatedAt: string
}

export interface Group {
  id: string
  name: string
  parentId?: string
  createdAt: string
  updatedAt: string
}

export interface RunSummary {
  total: number
  pending: number
  running: number
  success: number
  failed: number
  timeout: number
  cancelled: number
  lost: number
  skipped: number
}

export interface RunTarget {
  id: string
  runId: string
  hostId: string
  status: string
  exitCode?: number
  startedAt?: string
  endedAt?: string
  errorCode?: string
  errorText?: string
  stdoutPreview?: string
  stderrPreview?: string
  bytesOut: number
}

export interface Run {
  id: string
  jobId: string
  trigger: string
  status: string
  startedAt?: string
  endedAt?: string
  summary: RunSummary
  createdAt: string
  updatedAt: string
  targets?: RunTarget[]
}

export interface Job {
  id: string
  kind: 'adhoc' | 'scheduled'
  name: string
  command: string
  targetIds: string[]
  snippetId?: string
  scheduleExpr?: string
  enabled: boolean
  timeoutMs: number
  concurrency: number
  createdBy?: string
  lastRunAt?: string
  nextRunAt?: string
  createdAt: string
  updatedAt: string
}

export interface Snippet {
  id: string
  title: string
  body: string
  tags: string[]
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface Tunnel {
  id: string
  hostId: string
  kind: 'local' | 'remote'
  localHost: string
  localPort: number
  remoteHost: string
  remotePort: number
  startedBy?: string
  status: string
  startedAt?: string
  stoppedAt?: string
  lastError?: string
}

export interface AuditEntry {
  seq?: number
  eventId: string
  at: string
  actorId?: string
  actorName?: string
  action: string
  objectType?: string
  objectId?: string
  result: string
  detail?: unknown
  prevHash?: string
  hash: string
  ip?: string
}

export interface Agent {
  id: string
  name: string
  registeredAt?: string
  lastSeenAt?: string
  version?: string
  status: 'pending' | 'approved' | 'blocked'
  facts?: unknown
  online: boolean
}

export interface DirEntry {
  name: string
  size: number
  mode: number
  isDir: boolean
  mtime: number
}

export interface FileStat {
  name: string
  size: number
  mode: number
  isDir: boolean
  mtime: number
}

export interface Facts {
  hostname: string
  os: string
  kernel: string
  arch: string
  uptimeSec: number
  cpuModel: string
  cpuCores: number
  memTotal: number
  disk: { filesystem: string; mount: string; total: number; used: number; available: number }[]
}

export interface SessionView {
  id: string
  userId: string
  createdAt: string
  expiresAt: string
  lastSeenAt: string
  userAgent: string
  ip: string
  revoked: boolean
  current?: boolean
}

export interface DoctorCheck { name: string; ok: boolean; detail?: string }
export interface DoctorReport {
  ok: boolean
  version: string
  time: string
  checks: DoctorCheck[]
  coverage: string
}

export interface MetricPoint { at: string; value: number; extra?: string }
