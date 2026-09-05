import { useState } from 'react'
import { BackupApi } from '@/api/endpoints'
import { useToast } from '@/lib/toast'
import { PageTitle } from '@/components/Common'
import { ConfirmButton } from '@/components/Confirm'
import { download } from '@/lib/format'

// Backups are encrypted (argon2id KDF + AES-GCM) and must be inspected in a
// staging area before restore; restore requires an explicit confirm and the
// server keeps a safety copy of the current DB first.
export function BackupPage() {
  const toast = useToast()
  const [pass, setPass] = useState('')
  const [bundle, setBundle] = useState('')
  const [report, setReport] = useState<Record<string, number> | null>(null)

  async function doExport() {
    if (pass.length < 8) { toast.error('备份口令至少 8 位'); return }
    const r = await BackupApi.export(pass)
    const bytes = Uint8Array.from(atob(r.backupBase64), (c) => c.charCodeAt(0))
    download(`bastiondeck-backup-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.bak`, bytes)
    toast.success(`已导出加密备份（${r.bytes} 字节）`)
  }

  async function inspect() {
    try {
      const r = await BackupApi.inspect(bundle, pass)
      setReport(r.report.counts)
      toast.success('暂存校验通过，口令正确、内容完整')
    } catch (e) { toast.error(e instanceof Error ? e.message : '校验失败（口令错误或文件损坏）') }
  }

  function onFile(f: File) {
    const reader = new FileReader()
    reader.onload = () => {
      const bytes = new Uint8Array(reader.result as ArrayBuffer)
      let bin = ''
      bytes.forEach((b) => { bin += String.fromCharCode(b) })
      setBundle(btoa(bin))
    }
    reader.readAsArrayBuffer(f)
  }

  return (
    <>
      <div className="topbar"><h1>加密备份与恢复</h1></div>
      <div className="content">
        <div className="split">
          <div className="panel">
            <PageTitle title="导出" />
            <p className="muted">导出内容包含主机、凭据密文、任务、审计等逻辑表；用口令派生密钥做 AES-256-GCM 加密。</p>
            <label className="field"><span>备份口令</span>
              <input type="password" value={pass} onChange={(e) => setPass(e.target.value)} /></label>
            <button className="primary" onClick={doExport}>下载加密备份</button>
          </div>

          <div className="panel">
            <PageTitle title="恢复（先暂存校验，再确认覆盖）" />
            <label className="field"><span>选择 .bak 文件</span>
              <input type="file" onChange={(e) => { const f = e.target.files?.[0]; if (f) onFile(f) }} /></label>
            <label className="field"><span>备份口令</span>
              <input type="password" value={pass} onChange={(e) => setPass(e.target.value)} /></label>
            <div className="row">
              <button onClick={inspect}>暂存并校验</button>
              {report && (
                <ConfirmButton
                  danger
                  confirmText="我确认覆盖"
                  message="恢复将覆盖当前数据（服务端会先保留安全副本），确认继续？"
                  onConfirm={async () => {
                    const r = await BackupApi.restore(bundle, pass, true)
                    toast.success(`恢复完成，安全副本：${r.safetyCopy}`)
                  }}
                >
                  <button className="danger">校验通过，执行恢复</button>
                </ConfirmButton>
              )}
            </div>
            {report && (
              <pre className="mono panel" style={{ marginTop: 12 }}>
                {Object.entries(report).map(([k, v]) => `${k}: ${v}`).join('\n')}
              </pre>
            )}
          </div>
        </div>
      </div>
    </>
  )
}
