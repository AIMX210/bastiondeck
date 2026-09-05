import { useEffect, useRef } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { wsUrl } from '@/api/client'

// Interactive PTY over the /ws/term WebSocket protocol.
export function TerminalPane({ hostId }: { hostId: string }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!ref.current) return
    const term = new XTerm({
      fontFamily: 'ui-monospace, Menlo, Consolas, monospace',
      fontSize: 13,
      theme: { background: '#0a0e12', foreground: '#d7dde3', cursor: '#2dd4bf' },
      cursorBlink: true,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(ref.current)
    fit.fit()

    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const url = new URL(wsUrl(`/ws/term?hostId=${encodeURIComponent(hostId)}`))
    // cookie auth rides the handshake
    const ws = new WebSocket(`${proto}://${window.location.host}${url.pathname}${url.search}`)
    let opened = false

    ws.onopen = () => {
      const { cols, rows } = term
      ws.send(JSON.stringify({ type: 'open', hostId, cols, rows }))
      opened = true
    }
    ws.onmessage = (ev) => {
      try {
        const m = JSON.parse(ev.data)
        if (m.type === 'output') term.write(m.data)
        else if (m.type === 'closed') { term.write('\r\n\x1b[33m[session closed]\x1b[0m\r\n') }
        else if (m.type === 'error') term.write(`\r\n\x1b[31m[error] ${m.data}\x1b[0m\r\n`)
      } catch { /* ignore */ }
    }
    ws.onclose = () => term.write('\r\n\x1b[33m[disconnected]\x1b[0m\r\n')

    const dataDisp = term.onData((data) => {
      if (opened && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }))
      }
    })
    const onResize = () => {
      fit.fit()
      if (opened && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    window.addEventListener('resize', onResize)
    const ro = new ResizeObserver(onResize)
    ro.observe(ref.current)

    return () => {
      dataDisp.dispose()
      window.removeEventListener('resize', onResize)
      ro.disconnect()
      ws.close()
      term.dispose()
    }
  }, [hostId])

  return <div className="terminal-host" ref={ref} />
}
