// Package inventory owns the managed-host catalogue: hosts, groups, tags,
// favourites, OpenSSH config import and jump-host graph validation. It never
// opens network connections itself (architecture §3 dependency rule).
package inventory

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Host is a managed machine record.
type Host struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Address       string            `json:"address"`
	Port          int               `json:"port"`
	Username      string            `json:"username"`
	CredentialID  *string           `json:"credentialId,omitempty"`
	AuthKind      string            `json:"authKind"` // credential | agent
	AgentID       *string           `json:"agentId,omitempty"`
	JumpHostID    *string           `json:"jumpHostId,omitempty"`
	GroupID       *string           `json:"groupId,omitempty"`
	Tags          []string          `json:"tags"`
	Notes         string            `json:"notes"`
	Favorite      bool              `json:"favorite"`
	KnownHostKey  *string           `json:"knownHostKey,omitempty"`
	KnownKeyType  *string           `json:"knownHostKeyType,omitempty"`
	FirstSeenAt   *string           `json:"firstSeenAt,omitempty"`
	LastConnected *string           `json:"lastConnectedAt,omitempty"`
	LastStatus    string            `json:"lastStatus"`
	LastStatusAt  *string           `json:"lastStatusAt,omitempty"`
	Options       map[string]string `json:"options"`
	CreatedAt     string            `json:"createdAt"`
	UpdatedAt     string            `json:"updatedAt"`
}

// Group organises hosts hierarchically.
type Group struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ParentID  *string `json:"parentId,omitempty"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// Errors with stable codes consumed by the API layer.
var (
	ErrNotFound    = errors.New("host not found")
	ErrJumpCycle   = errors.New("jump cycle detected")
	ErrJumpTooDeep = errors.New("jump chain exceeds depth limit")
	ErrIsJumpHost  = errors.New("host is used as jump host")
	ErrBadAuthKind = errors.New("invalid auth kind")
)

// MaxJumpDepth is the inclusive maximum chain length.
const MaxJumpDepth = 5

func marshalTags(t []string) string {
	if t == nil {
		t = []string{}
	}
	b, _ := json.Marshal(t)
	return string(b)
}

func parseTags(s string) []string {
	var out []string
	if s == "" {
		return []string{}
	}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func marshalOptions(m map[string]string) string {
	if m == nil {
		m = map[string]string{}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func parseOptions(s string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func strPtr(v sql.NullString) *string {
	if v.Valid && v.String != "" {
		s := v.String
		return &s
	}
	return nil
}
