import { useState } from 'react'
import { AgentApi } from '@/api/endpoints'
import type { Agent } from '@/api/types'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { Empty, ErrorBox, Loading, PageTitle } from '@/components/Common'
import { StatusBadge } from '@/components/Badge'
import { Modal } from '@/components/Modal'
import { fmtRelative, shortId } from '@/lib/format'

export function AgentsPage() {
  const toast = useToast()
  const list = useAsync(() => AgentApi.list(), [], { intervalMs: 5_000 })
  const [enroll, setEnroll] = useState<{ id: string; secret: string; name: string } | null>(null)
  const [name, setName] = useState('')

  async function doEnroll() {
    try {
      const r = await AgentApi.enroll(name || 'new-agent')
      setEnroll({ id: r.agentId, secret: r.enrollSecret, name: name || 'new-agent' })
      list.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '注册失败') }
  }

  return (
    <>
      <div className="topbar"><h1>bd-agent 节点</h1></div>
      <div className="content">
        <ErrorBox message={list.error} />
        <PageTitle title="反向接入节点（出站 WebSocket，审批后才可执行）" extra={
          <div className="toolbar" style={{ margin: 0 }}>
            <input style={{ width: 200 }} placeholder="节点名" value={name} onChange={(e) => setName(e.target.value)} />
            <button className="primary" onClick={doEnroll}>注册并获取一次性密钥</button>
          </div>
        } />
        {list.loading ? <Loading /> : (list.data?.agents.length === 0) ? <Empty text="暂无 agent 节点" /> : (
          <div className="panel scroll-x">
            <table className="grid">
              <thead><tr><th>名称</th><th>ID</th><th>在线</th><th>审批状态</th><th>版本</th><th>最近心跳</th><th style={{ width: 180 }}>操作</th></tr></thead>
              <tbody>
                {(list.data?.agents ?? []).map((a: Agent) => (
                  <tr key={a.id}>
                    <td>{a.name}</td>
                    <td className="mono muted">{shortId(a.id)}</td>
                    <td><StatusBadge status={a.online ? 'online' : 'offline'} label={a.online ? '在线' : '离线'} /></td>
                    <td><StatusBadge status={a.status} /></td>
                    <td>{a.version ?? '—'}</td>
                    <td className="muted">{fmtRelative(a.lastSeenAt)}</td>
                    <td>
                      <div className="row">
                        {a.status !== 'approved' && <button className="sm primary" onClick={async () => {
                          await AgentApi.approve(a.id); list.reload(); toast.success('已批准')
                        }}>批准</button>}
                        {a.status !== 'blocked' && <button className="sm danger" onClick={async () => {
                          await AgentApi.block(a.id); list.reload(); toast.info('已拉黑')
                        }}>拉黑</button>}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {enroll && (
          <Modal title="Agent 注册信息（密钥仅显示一次）" onClose={() => setEnroll(null)}>
            <p className="muted">在目标机器上运行：</p>
            <pre className="mono panel" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
{`bd-agent --server https://<你的地址> \\
  --secret ${enroll.secret} \\
  --name ${enroll.name}`}
            </pre>
            <p className="err-text">关闭后无法再次查看该密钥，可重新注册。</p>
          </Modal>
        )}
      </div>
    </>
  )
}
