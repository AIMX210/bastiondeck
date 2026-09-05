// Package sshlite is the SSH backend for connector: bounded-time dialing,
// jump-host chains (≤5, acyclic), TOFU host-key handling, a connection pool
// and separated stdout/stderr execution.
package sshlite

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"

	"bastiondeck/internal/connector"
	"bastiondeck/internal/credentials"
	"bastiondeck/internal/inventory"
)

// Dialer builds raw *ssh.Client values for hosts.
type Dialer struct {
	Hosts *inventory.Repo
	Creds *credentials.Service

	// DialTimeout bounds the whole handshake of a single hop.
	DialTimeout time.Duration
}

// ErrHostKeyChanged is returned when a previously trusted key differs.
type ErrHostKeyChanged struct{ HostID, Want, Got string }

func (e *ErrHostKeyChanged) Error() string {
	return fmt.Sprintf("host key changed for %s: want %s got %s", e.HostID, e.Want, e.Got)
}

// Code returns the stable API error code.
func (e *ErrHostKeyChanged) Code() string { return "host_key_changed" }

// Connect resolves a host (and its jump chain) and returns a connector.Client.
func (d *Dialer) Connect(ctx context.Context, hostID string) (connector.Client, error) {
	h, err := d.Hosts.Get(ctx, hostID)
	if err != nil {
		return nil, err
	}
	raw, keyType, fp, err := d.dialRec(ctx, h, 0)
	if err != nil {
		return nil, err
	}
	return newClient(hostID, raw, keyType, fp), nil
}

func (d *Dialer) timeout() time.Duration {
	if d.DialTimeout > 0 {
		return d.DialTimeout
	}
	return 15 * time.Second
}

// dialRec dials h, recursively establishing jump hosts first.
func (d *Dialer) dialRec(ctx context.Context, h *inventory.Host, depth int) (*ssh.Client, string, string, error) {
	if depth > inventory.MaxJumpDepth {
		return nil, "", "", inventory.ErrJumpTooDeep
	}
	cfg, keyType, err := d.clientConfig(ctx, h)
	if err != nil {
		return nil, "", "", err
	}
	addr := net.JoinHostPort(h.Address, strconv.Itoa(h.Port))
	dialCtx, cancel := context.WithTimeout(ctx, d.timeout())
	defer cancel()

	var raw *ssh.Client
	if h.JumpHostID != nil && *h.JumpHostID != "" {
		jump, err := d.Hosts.Get(ctx, *h.JumpHostID)
		if err != nil {
			return nil, "", "", fmt.Errorf("jump host: %w", err)
		}
		jumpClient, _, _, err := d.dialRec(ctx, jump, depth+1)
		if err != nil {
			return nil, "", "", err
		}
		conn, err := jumpClient.Dial("tcp", addr)
		if err != nil {
			_ = jumpClient.Close()
			return nil, "", "", fmt.Errorf("dial via jump: %w", err)
		}
		// 握手超时看护：dialCtx 到期或父 ctx 取消时掐断连接；握手成功后
		// close(stop) 解除看护。此前无 stop 通道，看护 goroutine 会在
		// DialTimeout 到点时无条件 Close(conn)，把已建立的跳板连接误杀。
		stop := make(chan struct{})
		go func() {
			select {
			case <-dialCtx.Done():
				_ = conn.Close()
			case <-stop:
			}
		}()
		nc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
		if err != nil {
			close(stop)
			_ = conn.Close()
			_ = jumpClient.Close()
			return nil, "", "", fmt.Errorf("ssh over jump: %w", err)
		}
		close(stop)
		raw = ssh.NewClient(nc, chans, reqs)
	} else {
		nd := net.Dialer{Timeout: d.timeout()}
		conn, err := nd.DialContext(dialCtx, "tcp", addr)
		if err != nil {
			return nil, "", "", classifyDial(err)
		}
		if dl, ok := dialCtx.Deadline(); ok {
			_ = conn.SetDeadline(dl)
		}
		nc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
		if err != nil {
			_ = conn.Close()
			return nil, "", "", classifyDial(err)
		}
		_ = conn.SetDeadline(time.Time{})
		raw = ssh.NewClient(nc, chans, reqs)
	}
	// The TOFU callback has now recorded the key; read it back for the client.
	given, err := d.Hosts.Get(ctx, h.ID)
	if err == nil && given.KnownHostKey != nil {
		fp := *given.KnownHostKey
		kt := ""
		if given.KnownKeyType != nil {
			kt = *given.KnownKeyType
		}
		return raw, kt, fp, nil
	}
	return raw, keyType, "", nil
}

func (d *Dialer) clientConfig(ctx context.Context, h *inventory.Host) (*ssh.ClientConfig, string, error) {
	if h.AuthKind == "agent" {
		return nil, "", errors.New("agent-backed host must not use the SSH dialer")
	}
	var authMethods []ssh.AuthMethod
	if h.CredentialID != nil {
		secret, err := d.Creds.Reveal(ctx, *h.CredentialID)
		if err != nil {
			return nil, "", fmt.Errorf("credential: %w", err)
		}
		m, err := secret.AuthMethod()
		if err != nil {
			return nil, "", err
		}
		authMethods = append(authMethods, m)
	} else {
		// No credential: "none" auth is only valid against explicitly
		// permissive servers; still attempt keyboard-less none.
		authMethods = append(authMethods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
			return nil, nil
		}))
	}
	cb := &keyCallback{repo: d.Hosts, ctx: ctx, hostID: h.ID}
	cfg := &ssh.ClientConfig{
		User:            h.Username,
		Auth:            authMethods,
		Timeout:         d.timeout(),
		HostKeyCallback: cb.check,
	}
	return cfg, "", nil
}

// keyCallback implements TOFU via the inventory repository.
type keyCallback struct {
	repo   *inventory.Repo
	ctx    context.Context
	hostID string
}

func (k *keyCallback) String() string { return "tofu" }

func (k *keyCallback) check(hostname string, _ net.Addr, key ssh.PublicKey) error {
	fp := ssh.FingerprintSHA256(key)
	err := k.repo.RecordHostKey(k.ctx, k.hostID, key.Type(), fp)
	var changed *inventory.KeyChangedError
	if errors.As(err, &changed) {
		return &ErrHostKeyChanged{HostID: k.hostID, Want: changed.Want, Got: changed.Got}
	}
	return err
}

// classifyDial maps network errors to stable codes.
func classifyDial(err error) error {
	if err == nil {
		return nil
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return &DialError{Code: "conn_timeout", Err: err}
	}
	return &DialError{Code: "conn_failed", Err: err}
}

// DialError carries a stable code for the API layer.
type DialError struct {
	Code string
	Err  error
}

func (e *DialError) Error() string { return e.Err.Error() }
func (e *DialError) Unwrap() error { return e.Err }
