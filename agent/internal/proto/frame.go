// Package proto defines the JSON-frame wire protocol shared with the
// BastionDeck server. It is intentionally dependency-free so the agent stays
// a small static binary.
package proto

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

// Frame mirrors the server-side envelope.
type Frame struct {
	T           string          `json:"t"`
	ID          string          `json:"id,omitempty"`
	Secret      string          `json:"secret,omitempty"`
	Version     string          `json:"version,omitempty"`
	Command     string          `json:"command,omitempty"`
	TimeoutMs   int             `json:"timeoutMs,omitempty"`
	Stream      string          `json:"stream,omitempty"`
	DataB64     string          `json:"dataB64,omitempty"`
	Status      string          `json:"status,omitempty"`
	ExitCode    int             `json:"exitCode,omitempty"`
	ErrorCode   string          `json:"errorCode,omitempty"`
	ErrorText   string          `json:"errorText,omitempty"`
	Op          string          `json:"op,omitempty"`
	Path        string          `json:"path,omitempty"`
	Dest        string          `json:"dest,omitempty"`
	ContentB64  string          `json:"contentB64,omitempty"`
	ExpectedSHA string          `json:"expectedSha,omitempty"`
	Facts       json.RawMessage `json:"facts,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// Conn wraps a websocket with guarded writes.
type Conn struct {
	c       *websocket.Conn
	writeCh chan Frame
}

// Wrap builds a Conn around an upgraded socket and starts the write pump.
func Wrap(c *websocket.Conn) *Conn {
	co := &Conn{c: c, writeCh: make(chan Frame, 32)}
	go co.writePump()
	return co
}

func (c *Conn) writePump() {
	for f := range c.writeCh {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		b, _ := json.Marshal(f)
		_ = c.c.Write(ctx, websocket.MessageText, b)
		cancel()
	}
}

// Send enqueues a frame (never blocks the request loop).
func (c *Conn) Send(f Frame) {
	select {
	case c.writeCh <- f:
	default:
	}
}

// Read blocks for the next inbound frame.
func (c *Conn) Read(ctx context.Context) (Frame, error) {
	_, data, err := c.c.Read(ctx)
	if err != nil {
		return Frame{}, err
	}
	var f Frame
	err = json.Unmarshal(data, &f)
	return f, err
}

// Close terminates the socket and write pump.
func (c *Conn) Close() {
	close(c.writeCh)
	_ = c.c.Close(websocket.StatusNormalClosure, "")
}
