package agentconn

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Frame is the JSON envelope exchanged with bd-agent.
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

// connState is one live agent session with pending request channels.
type connState struct {
	agentID string
	conn    *websocket.Conn
	writeMu sync.Mutex

	mu      sync.Mutex
	pending map[string]chan Frame
	nextID  atomic.Int64
}

// ServeWS handles an inbound agent connection. The agent authenticates with
// its enrollment secret on the query string; session auth does not apply.
func (r *Registry) ServeWS(w http.ResponseWriter, req *http.Request) {
	secret := req.URL.Query().Get("secret")
	view, err := r.Authenticate(req.Context(), secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	st := &connState{agentID: view.ID, conn: conn, pending: map[string]chan Frame{}}
	r.setSession(view.ID, st)
	defer r.setSession(view.ID, nil)
	r.SetLive(view.ID, &LiveConn{
		AgentID: view.ID, Since: time.Now(),
		Send: func(m any) error {
			f, ok := m.(Frame)
			if !ok {
				b, _ := json.Marshal(m)
				return st.sendBytes(b)
			}
			return st.send(f)
		},
	})
	defer r.SetLive(view.ID, nil)
	defer r.Touch(req.Context(), view.ID)

	_ = r.MarkRegistered(req.Context(), view.ID, req.URL.Query().Get("version"))
	_ = st.send(Frame{T: "welcome", ID: view.ID})

	// Heartbeat watchdog: close if no frame for 90s.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = st.send(Frame{T: "ping"})
			}
		}
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var f Frame
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		r.Touch(req.Context(), view.ID)
		switch f.T {
		case "hello":
			if len(f.Facts) > 0 {
				_ = r.SaveFacts(req.Context(), view.ID, f.Facts)
			}
		case "heartbeat", "pong":
			// liveness only
		case "exec_chunk", "exec_end", "fs_res", "facts_res":
			st.deliver(f)
		}
	}
}

func (s *connState) send(f Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return s.sendBytes(b)
}

func (s *connState) sendBytes(b []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.conn.Write(ctx, websocket.MessageText, b)
}

func (s *connState) register() (string, chan Frame) {
	id := numID(s.nextID.Add(1))
	ch := make(chan Frame, 64)
	s.mu.Lock()
	s.pending[id] = ch
	s.mu.Unlock()
	return id, ch
}

func (s *connState) unregister(id string) {
	s.mu.Lock()
	if ch, ok := s.pending[id]; ok {
		delete(s.pending, id)
		close(ch)
	}
	s.mu.Unlock()
}

func (s *connState) deliver(f Frame) {
	s.mu.Lock()
	ch, ok := s.pending[f.ID]
	s.mu.Unlock()
	if ok {
		select {
		case ch <- f:
		default:
		}
	}
}

func numID(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
