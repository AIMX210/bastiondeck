package httpx

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// termMessage is the JSON wire protocol for /ws/term.
type termMessage struct {
	Type   string `json:"type"`             // open | input | resize | output | closed | error
	HostID string `json:"hostId,omitempty"` // open
	Cols   int    `json:"cols,omitempty"`
	Rows   int    `json:"rows,omitempty"`
	Data   string `json:"data,omitempty"`           // input（UTF-8 文本）/ output（base64，见 Enc）
	Enc    string `json:"enc,omitempty"`            // output 消息恒为 "base64"：PTY 字节流并非合法 UTF-8，
	                                              // 走 JSON string 会被替换为 U+FFFD 造成终端输出损坏
}

// terminalWS upgrades to a WebSocket, opens a PTY on the requested host and
// bridges bidirectional bytes until either side closes.
func (s *Server) terminalWS(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("hostId")
	if hostID == "" {
		writeErr(w, 400, "bad_request", "hostId required")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	// 不设置 OriginPatterns：coder/websocket 默认仅允许同源（+ 无 Origin 的
	// 原生客户端）。此前显式写 ["*"] 等于关闭跨站 WebSocket 劫持（CSWSH）
	// 防护——浏览器会自动携带 cookie，任意网页都能借此开终端。
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// First frame must be "open" with size.
	var open termMessage
	if err := readTerm(conn, ctx, &open, 10*time.Second); err != nil {
		termErr(conn, "handshake failed")
		return
	}
	cols, rows := open.Cols, open.Rows
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}

	cli, err := s.deps.Connector.Connect(ctx, hostID)
	if err != nil {
		termErr(conn, "connect: "+err.Error())
		return
	}
	defer cli.Close()
	pty, err := cli.OpenPty(ctx, cols, rows)
	if err != nil {
		termErr(conn, "pty: "+err.Error())
		return
	}
	defer pty.Close()
	_, _ = s.deps.Audit.Write(ctx, s.actorOf(r), "term.open", "host", hostID, "success", nil)

	var writeMu sync.Mutex
	send := func(m termMessage) {
		writeMu.Lock()
		defer writeMu.Unlock()
		b, _ := json.Marshal(m)
		_ = conn.Write(ctx, websocket.MessageText, b)
	}

	// Remote -> browser.
	go func() {
		buf := make([]byte, 8192)
		b64 := make([]byte, 0, 8192*4/3+4)
		for {
			n, err := pty.Read(buf)
			if n > 0 {
				b64 = b64[:0]
				b64 = base64.StdEncoding.AppendEncode(b64, buf[:n])
				send(termMessage{Type: "output", Data: string(b64), Enc: "base64"})
			}
			if err != nil {
				send(termMessage{Type: "closed"})
				cancel()
				return
			}
		}
	}()

	// Browser -> remote.
	for {
		var msg termMessage
		if err := readTerm(conn, ctx, &msg, 0); err != nil {
			return
		}
		switch msg.Type {
		case "input":
			if _, err := pty.Write([]byte(msg.Data)); err != nil {
				return
			}
		case "resize":
			_ = pty.Resize(msg.Cols, msg.Rows)
		case "ping":
			send(termMessage{Type: "pong"})
		}
	}
}

func readTerm(conn *websocket.Conn, ctx context.Context, dst *termMessage, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	_, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func termErr(conn *websocket.Conn, msg string) {
	b, _ := json.Marshal(termMessage{Type: "error", Data: msg})
	_ = conn.Write(context.Background(), websocket.MessageText, b)
}
