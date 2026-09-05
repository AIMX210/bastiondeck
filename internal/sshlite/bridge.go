package sshlite

import (
	"golang.org/x/crypto/ssh"

	"bastiondeck/internal/connector"
	"bastiondeck/internal/sftplite"
)

// FS opens an SFTP channel bound to this SSH connection.
func (c *Client) FS() (connector.FS, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return sftplite.NewFS(c.raw)
}

// SSH exposes the underlying ssh client for SSH-only subsystems (tunnels).
// Callers must not close it; the pool owns lifecycle.
func (c *Client) SSH() *ssh.Client { return c.raw }
