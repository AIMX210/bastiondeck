import { useState } from 'react'
import { SnippetApi } from '@/api/endpoints'
import type { Snippet } from '@/api/types'
import { useAsync } from '@/lib/hooks'
import { useToast } from '@/lib/toast'
import { useAuth } from '@/store/auth'
import { Empty, ErrorBox, Loading, PageTitle } from '@/components/Common'
import { Modal } from '@/components/Modal'
import { ConfirmButton } from '@/components/Confirm'

// Snippets use ${var} placeholders rendered server-side with explicit vars
// (no shell string interpolation surprises).
export function SnippetsPage() {
  const toast = useToast()
  const { can } = useAuth()
  const list = useAsync(() => SnippetApi.list(), [])
  const [editing, setEditing] = useState<Partial<Snippet> | null>(null)
  const [preview, setPreview] = useState<Snippet | null>(null)
  const writable = can('manage_inventory')

  async function save() {
    if (!editing) return
    if (!editing.title?.trim() || !editing.body?.trim()) { toast.error('标题和命令体不能为空'); return }
    try {
      if (editing.id) await SnippetApi.update(editing.id, {
        title: editing.title, body: editing.body, tags: editing.tags,
      })
      else await SnippetApi.create({
        title: editing.title!, body: editing.body!,
        tags: (editing.tags ?? []),
      })
      toast.success('已保存'); setEditing(null); list.reload()
    } catch (e) { toast.error(e instanceof Error ? e.message : '保存失败') }
  }

  return (
    <>
      <div className="topbar"><h1>命令片段</h1></div>
      <div className="content">
        <ErrorBox message={list.error} />
        <PageTitle title={'可复用命令（${变量} 占位，执行时显式填值）'} extra={
          writable && <button className="primary" onClick={() => setEditing({ title: '', body: '', tags: [] })}>+ 新建片段</button>
        } />
        {list.loading ? <Loading /> : (list.data?.snippets.length === 0) ? <Empty text="暂无片段" /> : (
          <div className="split">
            {(list.data?.snippets ?? []).map((s) => (
              <div className="panel" key={s.id}>
                <div className="toolbar" style={{ marginBottom: 8 }}>
                  <strong>{s.title}</strong>
                  <div className="grow" />
                  <button className="sm" onClick={() => setPreview(s)}>预览渲染</button>
                  {writable && <button className="sm" onClick={() => setEditing(s)}>编辑</button>}
                  {writable && (
                    <ConfirmButton danger message="删除片段？" onConfirm={async () => {
                      await SnippetApi.remove(s.id); list.reload()
                    }}><button className="sm danger">删</button></ConfirmButton>
                  )}
                </div>
                <pre className="mono" style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{s.body}</pre>
                <div className="muted" style={{ marginTop: 6 }}>{(s.tags ?? []).join(' · ')}</div>
              </div>
            ))}
          </div>
        )}

        {editing && (
          <Modal title={editing.id ? '编辑片段' : '新建片段'} onClose={() => setEditing(null)} width={680}
            footer={<><button onClick={() => setEditing(null)}>取消</button>
              <button className="primary" onClick={save}>保存</button></>}>
            <label className="field"><span>标题</span>
              <input value={editing.title ?? ''} onChange={(e) => setEditing({ ...editing, title: e.target.value })} /></label>
            <label className="field"><span>{'命令体（用 ${name} 表示变量）'}</span>
              <textarea rows={10} className="mono" value={editing.body ?? ''}
                onChange={(e) => setEditing({ ...editing, body: e.target.value })} /></label>
            <label className="field"><span>标签（逗号分隔）</span>
              <input value={(editing.tags ?? []).join(',')}
                onChange={(e) => setEditing({ ...editing, tags: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })} /></label>
          </Modal>
        )}
        {preview && <RenderPreview snippet={preview} onClose={() => setPreview(null)} />}
      </div>
    </>
  )
}

function RenderPreview({ snippet, onClose }: { snippet: Snippet; onClose: () => void }) {
  const toast = useToast()
  const [vars, setVars] = useState<Record<string, string>>({})
  const [out, setOut] = useState('')
  const names = Array.from(snippet.body.matchAll(/\$\{\s*([a-zA-Z0-9_.-]+)\s*\}/g)).map((m) => m[1])
  const uniq = Array.from(new Set(names))
  return (
    <Modal title={`渲染预览：${snippet.title}`} onClose={onClose}
      footer={<button className="primary" onClick={async () => {
        try {
          const r = await SnippetApi.render(snippet.body, vars)
          setOut(r.rendered + (r.missing.length ? `\n\n[缺失变量: ${r.missing.join(', ')}]` : ''))
        } catch (e) { toast.error(e instanceof Error ? e.message : '渲染失败') }
      }}>渲染</button>}>
      {uniq.map((n) => (
        <label className="field" key={n}><span>{n}</span>
          <input value={vars[n] ?? ''} onChange={(e) => setVars({ ...vars, [n]: e.target.value })} /></label>
      ))}
      {uniq.length === 0 && <div className="muted">该片段没有变量</div>}
      {out && <pre className="mono panel" style={{ whiteSpace: 'pre-wrap' }}>{out}</pre>}
    </Modal>
  )
}
