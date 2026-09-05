import { useMemo, useState } from 'react'
import { HostApi } from '@/api/endpoints'
import type { Group, Host } from '@/api/types'
import { useToast } from '@/lib/toast'
import { Modal } from './Modal'
import { ConfirmButton } from './Confirm'

// GroupsPanel manages host groups and shows live member counts. Deleting a
// group never deletes hosts: members simply fall back to "no group".
export function GroupsPanel({
  groups, hosts, reload, readOnly = false,
}: {
  groups: Group[]
  hosts: Host[]
  reload: () => void
  readOnly?: boolean
}) {
  const toast = useToast()
  const [creating, setCreating] = useState(false)
  const [renaming, setRenaming] = useState<Group | null>(null)
  const [name, setName] = useState('')

  const counts = useMemo(() => {
    const m = new Map<string, number>()
    hosts.forEach((h) => { if (h.groupId) m.set(h.groupId, (m.get(h.groupId) ?? 0) + 1) })
    return m
  }, [hosts])

  async function create() {
    try {
      await HostApi.createGroup(name)
      toast.success('分组已创建')
      setCreating(false); setName(''); reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '创建失败') }
  }

  async function rename() {
    if (!renaming) return
    try {
      await HostApi.renameGroup(renaming.id, name)
      toast.success('已重命名')
      setRenaming(null); reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '重命名失败') }
  }

  async function remove(g: Group) {
    try {
      await HostApi.removeGroup(g.id)
      toast.success('分组已删除（主机保留）')
      reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '删除失败') }
  }

  return (
    <div className="panel">
      <div className="toolbar" style={{ marginBottom: 10 }}>
        <h2 style={{ margin: 0 }}>分组（{groups.length}）</h2>
        <div className="grow" />
        {!readOnly && <button className="sm primary" onClick={() => { setName(''); setCreating(true) }}>+ 新建分组</button>}
      </div>
      {groups.length === 0 ? <div className="muted">暂无分组</div> : (
        <table className="grid">
          <thead><tr><th>名称</th><th>主机数</th>{!readOnly && <th style={{ width: 160 }}>操作</th>}</tr></thead>
          <tbody>
            {groups.map((g) => (
              <tr key={g.id}>
                <td>{g.name}</td>
                <td>{counts.get(g.id) ?? 0}</td>
                {readOnly ? null : (
                <td>
                  <div className="row">
                    <button className="sm" onClick={() => { setRenaming(g); setName(g.name) }}>重命名</button>
                    <ConfirmButton danger message="删除分组？组内主机不会被删除，仅变为未分组。"
                      onConfirm={() => remove(g)}>
                      <button className="sm danger">删除</button>
                    </ConfirmButton>
                  </div>
                </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {creating && (
        <Modal title="新建分组" onClose={() => setCreating(false)}
          footer={<><button onClick={() => setCreating(false)}>取消</button>
            <button className="primary" disabled={!name.trim()} onClick={create}>创建</button></>}>
          <input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="分组名，如 prod-web" />
        </Modal>
      )}
      {renaming && (
        <Modal title="重命名分组" onClose={() => setRenaming(null)}
          footer={<><button onClick={() => setRenaming(null)}>取消</button>
            <button className="primary" disabled={!name.trim()} onClick={rename}>保存</button></>}>
          <input autoFocus value={name} onChange={(e) => setName(e.target.value)} />
        </Modal>
      )}
    </div>
  )
}
