import { useCallback, useEffect, useRef, useState } from 'react'
import { FsApi, HostApi, utf8FromB64 } from '@/api/endpoints'
import type { DirEntry } from '@/api/types'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { ErrorBox, Loading, PageTitle } from '@/components/Common'
import { Modal } from '@/components/Modal'
import { ConfirmButton } from '@/components/Confirm'
import { fmtBytes, fmtTime, modeString } from '@/lib/format'

export function FilesPage() {
  const toast = useToast()
  const hosts = useAsync(() => HostApi.list(), [])
  const [hostId, setHostId] = useState('')
  const [path, setPath] = useState('/')
  const [editing, setEditing] = useState<{ path: string; sha?: string; content: string; readOnly?: boolean } | null>(null)
  const [mkdirAt, setMkdirAt] = useState(false)
  const [rename, setRename] = useState<DirEntry | null>(null)
  useEffect(() => {
    if (!hostId && hosts.data?.hosts[0]) setHostId(hosts.data.hosts[0].id)
  }, [hosts.data, hostId])

  const listing = useAsync(() => hostId ? FsApi.list(hostId, path) : Promise.resolve(null), [hostId, path])

  const go = useCallback((p: string) => {
    const norm = p.startsWith('/') ? p : `${path.replace(/\/$/, '')}/${p}`
    const clean = norm.replace(/\/+/g, '/')
    setPath(clean)
  }, [path])

  async function openFile(e: DirEntry) {
    try {
      const p = join(path, e.name)
      const r = await FsApi.read(hostId, p)
      // The server caps reads at 1 MiB: at/above that cap the content may be
      // truncated, so open read-only to avoid writing back a clipped file.
      // sha256 seeds the optimistic-concurrency check on save.
      setEditing({
        path: p,
        content: utf8FromB64(r.contentBase64),
        sha: r.sha256,
        readOnly: r.size >= 1_000_000,
      })
    } catch (err) { toast.error(err instanceof Error ? err.message : '读取失败（可能是二进制文件）') }
  }

  async function saveFile() {
    if (!editing) return
    try {
      await FsApi.write(hostId, editing.path, editing.content, editing.sha)
      toast.success('已保存（乐观并发：远端未被改动才写入）')
      setEditing(null)
      listing.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '保存失败') }
  }

  const fileRef = useRef<HTMLInputElement>(null)
  async function upload(f: File) {
    const buf = await f.arrayBuffer()
    try {
      await FsApi.write(hostId, join(path, f.name), buf)
      toast.success(`已上传 ${f.name}`)
      listing.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '上传失败') }
  }

  const crumbs = path.split('/').filter(Boolean)

  return (
    <>
      <div className="topbar"><h1>文件管理</h1></div>
      <div className="content">
        <ErrorBox message={listing.error} />
        <PageTitle title="" extra={
          <div className="toolbar" style={{ margin: 0 }}>
            <select style={{ width: 220 }} value={hostId} onChange={(e) => { setHostId(e.target.value); setPath('/') }}>
              {(hosts.data?.hosts ?? []).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
            </select>
            <button onClick={() => setPath('/')}>根目录</button>
            <button onClick={() => setMkdirAt(true)}>+ 目录</button>
            <button onClick={() => setEditing({ path: join(path, 'newfile.txt'), content: '' })}>+ 新建文件</button>
            <button onClick={() => fileRef.current?.click()}>上传</button>
            <input ref={fileRef} type="file" hidden onChange={(e) => {
              const f = e.target.files?.[0]; if (f) upload(f); e.target.value = ''
            }} />
          </div>
        } />
        <div className="panel">
          <div className="mono muted" style={{ marginBottom: 10 }}>
            <a onClick={() => setPath('/')} style={{ cursor: 'pointer' }}>/</a>
            {crumbs.map((c, i) => {
              const p = '/' + crumbs.slice(0, i + 1).join('/')
              return <span key={p}> <a onClick={() => setPath(p)} style={{ cursor: 'pointer' }}>{c}</a> /</span>
            })}
          </div>
          {listing.loading ? <Loading /> : (
            <table className="grid">
              <thead><tr><th>名称</th><th>权限</th><th>大小</th><th>修改时间</th><th style={{ width: 180 }}>操作</th></tr></thead>
              <tbody>
                {path !== '/' && (
                  <tr style={{ cursor: 'pointer' }} onClick={() => setPath('/' + crumbs.slice(0, -1).join('/'))}>
                    <td>..</td><td /><td /><td />
                  </tr>
                )}
                {(listing.data?.entries ?? []).map((e) => (
                  <tr key={e.name}>
                    <td style={{ cursor: e.isDir ? 'pointer' : 'default' }}
                      onClick={() => e.isDir && go(e.name)}>
                      {e.isDir ? '📁 ' : '📄 '}{e.name}
                    </td>
                    <td className="mono muted">{modeString(e.mode)}</td>
                    <td className="mono">{e.isDir ? '—' : fmtBytes(e.size)}</td>
                    <td className="muted">{fmtTime(new Date(e.mtime * 1000).toISOString())}</td>
                    <td>
                      <div className="row">
                        {!e.isDir && <button className="sm" onClick={() => openFile(e)}>编辑</button>}
                        <button className="sm" onClick={() => setRename(e)}>重命名</button>
                        <ConfirmButton danger message={`删除 ${e.name}？不可恢复`} onConfirm={async () => {
                          await FsApi.remove(hostId, join(path, e.name)); listing.reload(); toast.success('已删除')
                        }}><button className="sm danger">删</button></ConfirmButton>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        {editing && (
          <Modal title={editing.path} onClose={() => setEditing(null)} width={760}
            footer={<><button onClick={() => setEditing(null)}>取消</button>
              <button className="primary" disabled={editing.readOnly} onClick={saveFile}>保存</button></>}>
            <textarea rows={20} className="mono" value={editing.content}
              onChange={(e) => setEditing({ ...editing, content: e.target.value })} />
            {editing.readOnly && <div className="err-text">文件过大，只读</div>}
          </Modal>
        )}

        {mkdirAt && (
          <MkdirModal onClose={() => setMkdirAt(false)} onDone={(name) => {
            FsApi.mkdir(hostId, join(path, name)).then(() => { toast.success('已创建'); setMkdirAt(false); listing.reload() })
              .catch((e) => toast.error(e.message))
          }} />
        )}
        {rename && (
          <RenameModal entry={rename} onClose={() => setRename(null)} onDone={(to) => {
            FsApi.rename(hostId, join(path, rename.name), join(path, to)).then(() => {
              toast.success('已重命名'); setRename(null); listing.reload()
            }).catch((e) => toast.error(e.message))
          }} />
        )}
      </div>
    </>
  )
}

function join(dir: string, name: string): string {
  return `${dir.replace(/\/$/, '')}/${name}`.replace(/\/+/g, '/')
}

function MkdirModal({ onClose, onDone }: { onClose: () => void; onDone: (name: string) => void }) {
  const [name, setName] = useState('')
  return (
    <Modal title="新建目录" onClose={onClose}
      footer={<><button onClick={onClose}>取消</button>
        <button className="primary" disabled={!name} onClick={() => onDone(name)}>创建</button></>}>
      <input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="目录名" />
    </Modal>
  )
}

function RenameModal({ entry, onClose, onDone }: { entry: DirEntry; onClose: () => void; onDone: (to: string) => void }) {
  const [name, setName] = useState(entry.name)
  return (
    <Modal title={`重命名 ${entry.name}`} onClose={onClose}
      footer={<><button onClick={onClose}>取消</button>
        <button className="primary" onClick={() => onDone(name)}>确定</button></>}>
      <input autoFocus value={name} onChange={(e) => setName(e.target.value)} />
    </Modal>
  )
}
