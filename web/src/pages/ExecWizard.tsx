import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ExecApi, HostApi, RunApi, SnippetApi } from '@/api/endpoints'
import type { Host, RunTarget } from '@/api/types'
import { useAsync, useEventSource } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { ErrorBox, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { EventFeed } from '@/components/EventFeed'
import { shortId } from '@/lib/format'
import {
  collectTags, dedupeIds, renderTemplate, requiredVariables, selectHosts,
  type SelectionMode,
} from '@/lib/selection'

// Three-step batch execution wizard: pick targets → compose command → watch.
export function ExecWizardPage() {
  const toast = useToast()
  const navigate = useNavigate()
  const hosts = useAsync(() => HostApi.list(), [])
  const groups = useAsync(() => HostApi.groups(), [])
  const snippets = useAsync(() => SnippetApi.list(), [])
  const [step, setStep] = useState<1 | 2 | 3>(1)

  const [mode, setMode] = useState<SelectionMode>('all')
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [groupId, setGroupId] = useState('')
  const [tag, setTag] = useState('')

  const [snippetId, setSnippetId] = useState('')
  const [command, setCommand] = useState('')
  const [vars, setVars] = useState<Record<string, string>>({})
  const [timeoutSec, setTimeoutSec] = useState(60)
  const [runId, setRunId] = useState<string | null>(null)
  const [targets, setTargets] = useState<RunTarget[]>([])

  const allHosts = hosts.data?.hosts ?? []
  const allTags = useMemo(() => collectTags(allHosts), [allHosts])
  const groupName = useMemo(() => {
    const m = new Map<string, string>()
    ;(groups.data?.groups ?? []).forEach((g) => m.set(g.id, g.name))
    return m
  }, [groups.data])

  const selectedHosts = useMemo<Host[]>(() =>
    selectHosts(allHosts, { mode, picked, groupId, tag }),
  [mode, picked, groupId, tag, allHosts])

  const varNames = useMemo(() => {
    const body = snippetId ? (snippets.data?.snippets.find((s) => s.id === snippetId)?.body ?? command) : command
    return requiredVariables(body)
  }, [command, snippetId, snippets.data])

  async function launch() {
    if (!command.trim()) { toast.error('命令不能为空'); return }
    if (selectedHosts.length === 0) { toast.error('没有选中任何目标'); return }
    if (!timeoutSec || timeoutSec < 1) { toast.error('超时需为正整数（秒）'); return }
    // Block up-front when a ${var} was left unfilled, mirroring the server's
    // missing_vars rejection so we never fan out a half-rendered command.
    const { missing } = renderTemplate(command, vars)
    if (missing.length > 0) { toast.error(`请先填写变量：${missing.join(', ')}`); return }
    try {
      const r = await ExecApi.run({
        command,
        targetIds: dedupeIds(selectedHosts.map((h) => h.id)),
        snippetId: snippetId || undefined,
        timeoutSec,
        vars,
      })
      setRunId(r.runId)
      setStep(3)
      setTargets([])
      toast.success(`已扇出到 ${selectedHosts.length} 台主机`)
      // Fetch immediately instead of relying solely on the first SSE event.
      refreshRun(r.runId)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '发起失败')
    }
  }

  async function refreshRun(id: string) {
    try {
      const r = await RunApi.get(id)
      setTargets(r.run.targets ?? [])
    } catch { /* transient */ }
  }
  useEventSource((_t, d) => {
    const dd = d as { runId?: string }
    if (runId && dd.runId === runId) refreshRun(runId)
  })

  function toggle(id: string) {
    setPicked((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }

  return (
    <>
      <div className="topbar"><h1>批量执行向导</h1></div>
      <div className="content">
        <ErrorBox message={hosts.error} />
        <div className="pill-tabs">
          {([[1, '1 选择目标'], [2, '2 编写命令'], [3, '3 实时观察']] as const).map(([n, l]) => (
            <button key={n} className={step === n ? 'active' : ''} onClick={() => setStep(n)}>{l}</button>
          ))}
        </div>

        {step === 1 && (
          <div className="panel">
            <PageTitle title={`目标选择（当前命中 ${selectedHosts.length} 台）`} />
            <div className="row" style={{ marginBottom: 12 }}>
              {([
                ['all', '全部主机'], ['pick', '手动勾选'], ['group', '按分组'], ['tag', '按标签'],
              ] as const).map(([v, l]) => (
                <button key={v} className={mode === v ? 'primary' : ''} onClick={() => setMode(v)}>{l}</button>
              ))}
              {mode === 'group' && (
                <select style={{ width: 200 }} value={groupId} onChange={(e) => setGroupId(e.target.value)}>
                  <option value="">选择分组</option>
                  {(groups.data?.groups ?? []).map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
                </select>
              )}
              {mode === 'tag' && (
                <select style={{ width: 200 }} value={tag} onChange={(e) => setTag(e.target.value)}>
                  <option value="">选择标签</option>
                  {allTags.map((t) => <option key={t}>{t}</option>)}
                </select>
              )}
            </div>
            <table className="grid">
              <thead><tr><th style={{ width: 40 }}></th><th>名称</th><th>地址</th><th>分组</th><th>标签</th></tr></thead>
              <tbody>
                {allHosts.map((h) => {
                  const hit = selectedHosts.some((x) => x.id === h.id)
                  return (
                    <tr key={h.id} style={{ opacity: mode === 'pick' || hit ? 1 : 0.45 }}
                      onClick={() => mode === 'pick' && toggle(h.id)}>
                      <td>
                        <input type="checkbox" style={{ width: 'auto' }} readOnly={mode !== 'pick'}
                          checked={hit} onChange={() => mode === 'pick' && toggle(h.id)} />
                      </td>
                      <td>{h.name}</td>
                      <td className="mono">{h.address}:{h.port}</td>
                      <td className="muted">{h.groupId ? (groupName.get(h.groupId) ?? h.groupId) : '—'}</td>
                      <td className="muted">{(h.tags ?? []).join(', ')}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
            <div className="modal-actions">
              <button className="primary" disabled={selectedHosts.length === 0}
                onClick={() => setStep(2)}>下一步：{selectedHosts.length} 台</button>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="panel">
            <PageTitle title="命令编写" />
            <label className="field"><span>从片段载入</span>
              <select value={snippetId} onChange={(e) => {
                setSnippetId(e.target.value)
                const s = snippets.data?.snippets.find((x) => x.id === e.target.value)
                if (s) setCommand(s.body)
              }}>
                <option value="">不使用片段，手写命令</option>
                {(snippets.data?.snippets ?? []).map((s) => <option key={s.id} value={s.id}>{s.title}</option>)}
              </select></label>
            <label className="field"><span>{'命令（${变量} 会在下方显式填值，绝不做隐式拼接）'}</span>
              <textarea rows={8} className="mono" value={command} onChange={(e) => setCommand(e.target.value)}
                placeholder="uptime; df -h /" /></label>
            {varNames.length > 0 && (
              <div className="split">
                {varNames.map((n) => (
                  <label className="field" key={n}><span>{n}</span>
                    <input value={vars[n] ?? ''} onChange={(e) => setVars({ ...vars, [n]: e.target.value })} /></label>
                ))}
              </div>
            )}
            <label className="field" style={{ maxWidth: 240 }}><span>单目标超时（秒）</span>
              <input type="number" value={timeoutSec} onChange={(e) => setTimeoutSec(Number(e.target.value))} /></label>
            <div className="modal-actions">
              <button onClick={() => setStep(1)}>上一步</button>
              <button className="primary" onClick={launch}>在 {selectedHosts.length} 台执行</button>
            </div>
          </div>
        )}

        {step === 3 && runId && (
          <>
            <div className="panel">
              <PageTitle title={<>运行 <span className="mono">{shortId(runId, 18)}</span></>} extra={
                <div className="row">
                  <button onClick={() => refreshRun(runId)}>刷新</button>
                  <button onClick={() => navigate(`/runs/${runId}`)}>进入详情</button>
                </div>
              } />
              <table className="grid">
                <thead><tr><th>主机</th><th>状态</th><th>退出码</th><th>stdout 预览</th></tr></thead>
                <tbody>
                  {targets.map((t) => {
                    const h = allHosts.find((x) => x.id === t.hostId)
                    return (
                      <tr key={t.id}>
                        <td>{h?.name ?? shortId(t.hostId)}</td>
                        <td><StatusBadge status={t.status} /></td>
                        <td className="mono">{t.exitCode ?? '—'}</td>
                        <td className="mono muted" style={{ maxWidth: 480, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {t.stdoutPreview ?? ''}
                        </td>
                      </tr>
                    )
                  })}
                  {targets.length === 0 && <tr><td colSpan={4} className="muted">调度中…</td></tr>}
                </tbody>
              </table>
            </div>
            <EventFeed />
          </>
        )}
      </div>
    </>
  )
}
