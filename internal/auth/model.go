// Package auth implements users, roles, password hashing, TOTP, sessions and
// the sliding-window login rate limiter.
package auth

import (
	"database/sql"
	"errors"
	"time"
)

// Role constants, ordered by privilege (owner is highest).
const (
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
	RoleOwner    = "owner"
)

// ErrBadCredentials is returned for any failed login (same error to avoid
// user enumeration).
var ErrBadCredentials = errors.New("invalid username or password")

// ErrLocked is returned while the login rate limiter is tripped.
var ErrLocked = errors.New("too many attempts, try again later")

// User is the stored user record.
type User struct {
	ID                 string  `json:"id"`
	Username           string  `json:"username"`
	DisplayName        string  `json:"displayName"`
	Role               string  `json:"role"`
	TOTPEnabled        bool    `json:"totpEnabled"`
	Disabled           bool    `json:"disabled"`
	MustChangePassword bool    `json:"mustChangePassword"`
	LastLoginAt        *string `json:"lastLoginAt,omitempty"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

// Session is a stored session row; the plaintext token is only known to the
// client (we store its SHA-256 digest).
type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
	UserAgent string
	IP        string
	Revoked   bool
}

// rank maps a role to a comparable privilege level.
func rank(role string) int {
	switch role {
	case RoleViewer:
		return 1
	case RoleOperator:
		return 2
	case RoleAdmin:
		return 3
	case RoleOwner:
		return 4
	default:
		return 0
	}
}

// ValidRole reports whether r is a known role.
func ValidRole(r string) bool { return rank(r) > 0 }

// AtLeast reports whether role satisfies required.
func AtLeast(role, required string) bool { return rank(role) >= rank(required) }

// Permission is an action category used by RBAC middleware and services.
type Permission string

const (
	PermRead            Permission = "read"
	PermExec            Permission = "exec"
	PermManageInventory Permission = "manage_inventory"
	PermAudit           Permission = "audit"
	PermManageUsers     Permission = "manage_users"
	PermOwner           Permission = "owner"
)

// Can is the single RBAC decision point.
func Can(role string, p Permission) bool {
	switch p {
	case PermRead:
		return rank(role) >= rank(RoleViewer)
	case PermExec:
		return rank(role) >= rank(RoleOperator)
	case PermManageInventory, PermAudit:
		return rank(role) >= rank(RoleAdmin)
	case PermManageUsers:
		// Only owner creates/modifies admins; admins manage operator/viewer.
		return rank(role) >= rank(RoleAdmin)
	case PermOwner:
		return role == RoleOwner
	}
	return false
}

// CanAssignRole reports whether actor may grant/change someone to targetRole.
func CanAssignRole(actorRole, targetRole string) bool {
	if !ValidRole(targetRole) {
		return false
	}
	if targetRole == RoleOwner {
		return actorRole == RoleOwner
	}
	if targetRole == RoleAdmin {
		return actorRole == RoleOwner
	}
	return rank(actorRole) >= rank(RoleAdmin) && rank(actorRole) > rank(targetRole)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func strFromNull(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}
