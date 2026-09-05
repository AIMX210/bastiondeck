import { api } from './client'
import type {
  Agent, AuditEntry, Credential, DoctorReport, Facts, Group, Host, Job, Run, SessionView,
  Snippet, Tunnel, User, DirEntry, FileStat, MetricPoint,
} from './types'

export const StatusApi = {
  status: () => api.get<{ version: string; setupRequired: boolean }>('/api/status'),
  health: () => api.get<{ ok: boolean; time: string }>('/api/healthz'),
}

export const AuthApi = {
  setup: (b: { username: string; password: string; displayName?: string }) =>
    api.post<{ user: User }>('/api/setup', b),
  login: (b: { username: string; password: string; totp?: string }) =>
    api.post<{ user: User; token: string }>('/api/auth/login', b),
  logout: () => api.post('/api/auth/logout'),
  me: () => api.get<{ user: User; permissions: string[] }>('/api/auth/me'),
  totpSetup: () => api.post<{ secret: string; uri: string }>('/api/auth/totp/setup'),
  totpEnable: (code: string) => api.post('/api/auth/totp/enable', { code }),
  changePassword: (oldPassword: string, newPassword: string) =>
    api.post('/api/auth/password', { oldPassword, newPassword }),
}

export const UsersApi = {
  list: () => api.get<{ users: User[] }>('/api/users'),
  create: (b: { username: string; displayName?: string; role: string; password: string }) =>
    api.post<{ user: User }>('/api/users', b),
  update: (id: string, b: Partial<{ role: string; displayName: string; disabled: boolean }>) =>
    api.patch<{ user: User }>(`/api/users/${id}`, b),
  remove: (id: string) => api.del(`/api/users/${id}`),
  sessions: (id: string) => api.get<{ sessions: SessionView[] }>(`/api/users/${id}/sessions`),
  mySessions: () => api.get<{ sessions: SessionView[] }>('/api/sessions'),
  revoke: (id: string) => api.del(`/api/sessions/${id}`),
  revokeAll: () => api.post<{ revoked: number }>('/api/sessions/revoke-all'),
}

export const CredApi = {
  list: () => api.get<{ credentials: Credential[] }>('/api/credentials'),
  create: (b: { name: string; kind: string; secret: string; passphrase?: string }) =>
    api.post<{ credential: Credential }>('/api/credentials', b),
  update: (id: string, b: { name?: string; secret?: string; passphrase?: string }) =>
    api.patch<{ credential: Credential }>(`/api/credentials/${id}`, b),
  remove: (id: string) => api.del(`/api/credentials/${id}`),
}

export const HostApi = {
  list: (q?: Record<string, string>) => api.get<{ hosts: Host[] }>('/api/hosts', q),
  get: (id: string) => api.get<{ host: Host }>(`/api/hosts/${id}`),
  create: (b: Partial<Host> & { address: string; username: string }) =>
    api.post<{ host: Host }>('/api/hosts', b),
  update: (id: string, b: Partial<Host>) => api.patch<{ host: Host }>(`/api/hosts/${id}`, b),
  remove: (id: string) => api.del(`/api/hosts/${id}`),
  test: (id: string) => api.post<Record<string, unknown>>(`/api/hosts/${id}/test`),
  resetKey: (id: string) => api.post(`/api/hosts/${id}/reset-host-key`),
  facts: (id: string) => api.post<{ facts: Facts }>(`/api/hosts/${id}/facts`),
  importSSHConfig: (text: string, credentialId?: string, defaultUser?: string) =>
    api.post<{ imported: unknown[]; count: number }>('/api/hosts/import-sshconfig',
      { text, credentialId, defaultUser }),
  groups: () => api.get<{ groups: Group[] }>('/api/groups'),
  createGroup: (name: string, parentId?: string) =>
    api.post<{ group: Group }>('/api/groups', { name, parentId }),
  renameGroup: (id: string, name: string) => api.patch(`/api/groups/${id}`, { name }),
  removeGroup: (id: string) => api.del(`/api/groups/${id}`),
}

export const ExecApi = {
  run: (b: { command: string; targetIds: string[]; groupId?: string; timeoutSec?: number; snippetId?: string; vars?: Record<string,string> }) =>
    api.post<{ runId: string }>('/api/exec', b),
  runJob: (jobId: string) => api.post<{ runId: string }>('/api/jobs/run', { jobId }),
}

export const JobApi = {
  list: () => api.get<{ jobs: Job[] }>('/api/jobs'),
  create: (b: Partial<Job>) => api.post<{ job: Job }>('/api/jobs', b),
  update: (id: string, b: Partial<Job>) => api.patch<{ job: Job }>(`/api/jobs/${id}`, b),
  remove: (id: string) => api.del(`/api/jobs/${id}`),
}

export const RunApi = {
  list: (limit = 50, cursor?: number) =>
    api.get<{ runs: Run[]; nextCursor?: number }>('/api/runs', { limit, cursor }),
  get: (id: string) => api.get<{ run: Run; live: boolean }>(`/api/runs/${id}`),
  output: (runId: string, targetId: string, stream: 'stdout' | 'stderr', offset: number) =>
    api.get<{ chunk: string; offset: number }>(`/api/runs/${runId}/targets/${targetId}/output`,
      { stream, offset }),
  cancel: (id: string) => api.post(`/api/runs/${id}/cancel`),
  retry: (id: string) => api.post<{ runId: string }>(`/api/runs/${id}/retry-failed`),
}

export const FsApi = {
  list: (hostId: string, path: string) =>
    api.get<{ path: string; entries: DirEntry[] }>('/api/fs/list', { hostId, path }),
  stat: (hostId: string, path: string) =>
    api.post<{ stat: FileStat }>('/api/fs/stat', { hostId, path }),
  read: (hostId: string, path: string, limit = 1_000_000) =>
    api.get<{ path: string; size: number; sha256?: string; contentBase64: string }>('/api/fs/read', { hostId, path, limit }),
  write: (hostId: string, path: string, content: ArrayBuffer | string, expectedSha?: string) => {
    const contentBase64 = typeof content === 'string'
      ? b64FromUtf8(content)
      : b64FromBytes(new Uint8Array(content))
    return api.post<{ sha256: string }>('/api/fs/write', { hostId, path, contentBase64, expectedSha })
  },
  mkdir: (hostId: string, path: string) => api.post('/api/fs/mkdir', { hostId, path }),
  rename: (hostId: string, from: string, to: string) =>
    api.post('/api/fs/rename', { hostId, from, to }),
  remove: (hostId: string, path: string) => api.post('/api/fs/delete', { hostId, path }),
}

export const TunnelApi = {
  list: () => api.get<{ tunnels: Tunnel[] }>('/api/tunnels'),
  create: (b: Partial<Tunnel>) => api.post<{ tunnel: Tunnel }>('/api/tunnels', b),
  stop: (id: string) => api.post(`/api/tunnels/${id}/stop`),
}

export const SnippetApi = {
  list: () => api.get<{ snippets: Snippet[] }>('/api/snippets'),
  create: (b: { title: string; body: string; tags?: string[] }) =>
    api.post<{ snippet: Snippet }>('/api/snippets', b),
  update: (id: string, b: { title?: string; body?: string; tags?: string[] }) =>
    api.patch<{ snippet: Snippet }>(`/api/snippets/${id}`, b),
  remove: (id: string) => api.del(`/api/snippets/${id}`),
  render: (body: string, vars: Record<string, string>) =>
    api.post<{ rendered: string; missing: string[] }>('/api/snippets/render', { body, vars }),
}

export const AuditApi = {
  list: (q: Record<string, string|number|undefined> = {}) =>
    api.get<{ entries: AuditEntry[]; nextCursor?: number }>('/api/audit', q),
  verify: () => api.post<{ chain: { ok: boolean; checked: number; brokenAt?: number; reason?: string } }>('/api/audit/verify'),
}

export const SettingsApi = {
  get: () => api.get<{ settings: Record<string, string> }>('/api/settings'),
  put: (b: Record<string, string>) => api.put<{ settings: Record<string, string> }>('/api/settings', b),
}

export const BackupApi = {
  export: (passphrase: string) =>
    api.post<{ backupBase64: string; bytes: number }>('/api/backup/export', { passphrase }),
  inspect: (backupBase64: string, passphrase: string) =>
    api.post<{ report: { version: number; at: string; counts: Record<string, number> } }>(
      '/api/backup/inspect', { backupBase64, passphrase }),
  restore: (backupBase64: string, passphrase: string, confirm: boolean) =>
    api.post<{ ok: boolean; safetyCopy: string }>('/api/backup/restore',
      { backupBase64, passphrase, confirm }),
}

export const AgentApi = {
  list: () => api.get<{ agents: Agent[] }>('/api/agents'),
  enroll: (name: string) => api.post<{ agentId: string; enrollSecret: string }>('/api/agents/enroll', { name }),
  approve: (id: string) => api.post(`/api/agents/${id}/approve`),
  block: (id: string) => api.post(`/api/agents/${id}/block`),
}

export const MetricsApi = {
  host: (id: string, kind?: string, from?: string, to?: string) =>
    api.get<unknown>(`/api/metrics/hosts/${id}`, { kind, from, to }) as Promise<{
      series?: Record<string, MetricPoint[]>
      kind?: string
      points?: MetricPoint[]
    }>,
}

export const DoctorApi = {
  run: () => api.get<DoctorReport>('/api/doctor'),
}

// ---- base64 helpers ----
export function b64FromUtf8(s: string): string {
  return btoa(unescape(encodeURIComponent(s)))
}
export function utf8FromB64(b64: string): string {
  try { return decodeURIComponent(escape(atob(b64))) } catch { return '' }
}
function b64FromBytes(bytes: Uint8Array): string {
  let bin = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk))
  }
  return btoa(bin)
}
