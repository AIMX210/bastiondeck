// Package testutil provides deterministic in-process fixtures used across
// packages: a scriptable SSH server, a temp SQLite store and credential
// generators. Nothing here is linked into production binaries (it is only
// imported by _test.go files and the e2e harness).
package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ExecHandler maps a command to stdout, stderr and exit code.
type ExecHandler func(cmd string) (stdout, stderr []byte, exitCode int)

// FakeSSH is an in-process SSH server for tests.
type FakeSSH struct {
	Listener net.Listener
	Signer   ssh.Signer
	pubFP    string

	User     string
	Password string

	mu       sync.Mutex
	exec     ExecHandler
	sftpRoot string
	closed   chan struct{}
	conns    []net.Conn
}

// NewFakeSSH starts a loopback server. sftpRoot enables the sftp subsystem
// backed by a local directory (empty disables SFTP).
func NewFakeSSH(t testing.TB, password, sftpRoot string, exec ExecHandler) *FakeSSH {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	cfg := &ssh.ServerConfig{
		MaxAuthTries: 5,
		PasswordCallback: func(_ ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if password != "" && string(pw) != password {
				return nil, errors.New("bad password")
			}
			return &ssh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &FakeSSH{
		Listener: ln, Signer: signer, User: "tester", Password: password,
		exec: exec, sftpRoot: sftpRoot, closed: make(chan struct{}),
	}
	f.pubFP = ssh.FingerprintSHA256(signer.PublicKey())
	go f.serveLoop(cfg)
	return f
}

// Addr returns host, port of the listener.
func (f *FakeSSH) Addr() (string, int) {
	host, p, _ := net.SplitHostPort(f.Listener.Addr().String())
	port, _ := strconv.Atoi(p)
	return host, port
}

// Fingerprint returns the SHA256 fingerprint clients should trust.
func (f *FakeSSH) Fingerprint() string { return f.pubFP }

// SetExec replaces the exec handler (test scripting).
func (f *FakeSSH) SetExec(h ExecHandler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exec = h
}

func (f *FakeSSH) serveLoop(cfg *ssh.ServerConfig) {
	for {
		conn, err := f.Listener.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns = append(f.conns, conn)
		f.mu.Unlock()
		go f.handleConn(conn, cfg)
	}
}

func (f *FakeSSH) handleConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		go f.handleSession(ch, chReqs)
	}
}

func (f *FakeSSH) handleSession(ch ssh.Channel, in <-chan *ssh.Request) {
	defer ch.Close()
	for req := range in {
		switch req.Type {
		case "exec":
			cmd := parseExec(req.Payload)
			f.mu.Lock()
			h := f.exec
			f.mu.Unlock()
			var stdout, stderr []byte
			code := 0
			if h != nil {
				stdout, stderr, code = h(cmd)
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			if len(stdout) > 0 {
				_, _ = ch.Write(stdout)
			}
			if len(stderr) > 0 {
				_, _ = ch.Stderr().Write(stderr)
			}
			_ = sendExitStatus(ch, code)
			return
		case "subsystem":
			name := parseSubsystem(req.Payload)
			if name == "sftp" && f.sftpRoot != "" {
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
				srv := sftp.NewRequestServer(ch, sftp.Handlers{
					FileGet:  &diskFileGet{root: f.sftpRoot},
					FilePut:  &diskFilePut{root: f.sftpRoot},
					FileCmd:  &diskCmd{root: f.sftpRoot},
					FileList: &diskList{root: f.sftpRoot},
				})
				_ = srv.Serve()
				return
			}
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		case "pty-req", "shell":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			if req.Type == "shell" {
				go func() { _, _ = io.Copy(ch, ch) }()
			}
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func parseExec(payload []byte) string {
	var s struct {
		Command string
	}
	_ = ssh.Unmarshal(payload, &s)
	return s.Command
}

func parseSubsystem(payload []byte) string {
	var s struct {
		Name string
	}
	_ = ssh.Unmarshal(payload, &s)
	return s.Name
}

func sendExitStatus(ch ssh.Channel, code int) error {
	msg := struct {
		Status uint32
	}{Status: uint32(code)}
	_, err := ch.SendRequest("exit-status", false, ssh.Marshal(msg))
	return err
}

// Close stops the server.
func (f *FakeSSH) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	f.mu.Lock()
	for _, c := range f.conns {
		_ = c.Close()
	}
	f.mu.Unlock()
	return f.Listener.Close()
}

// MustGeneratePrivateKey returns a fresh PEM-encoded private key.
func MustGeneratePrivateKey(t testing.TB) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

// Sleep tolerates flaky scheduling in tests.
func Sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

// safeJoin prevents path traversal outside the SFTP root.
func safeJoin(root, p string) string {
	p = strings.TrimPrefix(p, "/")
	return filepath.Join(root, filepath.Clean("/"+p))
}
