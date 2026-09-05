package sshlite

import (
	"context"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"

	"bastiondeck/internal/connector"
)

// ptySession wraps an SSH PTY session as connector.PtySession.
type ptySession struct {
	sess   *ssh.Session
	stdin  io.WriteCloser
	stdout io.Reader
	closeM sync.Once
	closeE error
}

// OpenPty starts an interactive shell with a pseudo terminal. With a PTY the
// remote side merges stderr into the same channel as stdout.
func (c *Client) OpenPty(_ context.Context, cols, rows int) (connector.PtySession, error) {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}
	sess, err := c.raw.NewSession()
	if err != nil {
		return nil, err
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = sess.Close()
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		_ = sess.Close()
		return nil, err
	}
	return &ptySession{sess: sess, stdin: stdin, stdout: stdout}, nil
}

func (p *ptySession) Read(b []byte) (int, error)  { return p.stdout.Read(b) }
func (p *ptySession) Write(b []byte) (int, error) { return p.stdin.Write(b) }

// Resize sends a window-change request.
func (p *ptySession) Resize(cols, rows int) error {
	return p.sess.WindowChange(rows, cols)
}

// Wait blocks until the remote shell exits.
func (p *ptySession) Wait() error { return p.sess.Wait() }

// Close terminates the PTY session exactly once.
func (p *ptySession) Close() error {
	p.closeM.Do(func() {
		_ = p.stdin.Close()
		p.closeE = p.sess.Close()
	})
	return p.closeE
}
