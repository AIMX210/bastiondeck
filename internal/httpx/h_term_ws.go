package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"bastiondeck/internal/audit"
)

// termMessage is the JSON wire protocol for /ws/term.
type termMessage struct {
	Type   string `json:"type"`             // open | input | resize | output | closed | error
	HostID string `json:"hostId,omitempty"` // open
	Cols   int    `json:"cols,omitempty"`
	Rows   int    `json:"rows,omitempty"`
	Data   string `json:"data,omitempty"` // input/output as base64? we use latin1-safe string
}

// terminalWS upgrades to a WebSocket, opens a PTY on the requested host and
// bridges bidirectional bytes until either side closes.
func (s *Server) terminalWS(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("hostId")
	if hostID == "" {
		writeErr(w, 400, "bad_request", "hostId required")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
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
		for {
			n, err := pty.Read(buf)
			if n > 0 {
				send(termMessage{Type: "output", Data: string(buf[:n])})
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

var _ = audit.Actor{}
