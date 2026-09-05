package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"bastiondeck/internal/store"
)

// CreateUserInput carries fields needed to create a user.
type CreateUserInput struct {
	Username    string
	DisplayName string
	Role        string
	Password    string
}

// CountUsers returns the number of users (bootstrap gate).
func (s *Service) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser hashes the password and inserts a user. Caller is responsible
// for RBAC (CanAssignRole) and bootstrap rules.
func (s *Service) CreateUser(ctx context.Context, in CreateUserInput) (*User, error) {
	in.Username = strings.TrimSpace(in.Username)
	if len(in.Username) < 3 {
		return nil, errors.New("username must be at least 3 characters")
	}
	if !ValidRole(in.Role) {
		return nil, fmt.Errorf("invalid role %q", in.Role)
	}
	if probs := PasswordStrength(in.Password); len(probs) > 0 {
		return nil, fmt.Errorf("weak password: %s", strings.Join(probs, ", "))
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u := &User{
		ID:          store.NewID(store.PrefixUser),
		Username:    in.Username,
		DisplayName: in.DisplayName,
		Role:        in.Role,
		CreatedAt:   store.Now(),
		UpdatedAt:   store.Now(),
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO users
        (id,username,display_name,role,password_hash,created_at,updated_at)
        VALUES(?,?,?,?,?,?,?)`,
		u.ID, u.Username, u.DisplayName, u.Role, hash, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("username already exists")
		}
		return nil, err
	}
	return u, nil
}

// GetUserByID loads a user without secret fields.
func (s *Service) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,username,display_name,role,totp_enabled,
        disabled,must_change_password,last_login_at,created_at,updated_at FROM users WHERE id=?`, id)
	return scanUser(row)
}

// ListUsers returns all users ordered by username.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,username,display_name,role,totp_enabled,
        disabled,must_change_password,last_login_at,created_at,updated_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// rowScanner abstracts *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(sc rowScanner) (*User, error) {
	var u User
	var totp, disabled, must int
	var last sql.NullString
	err := sc.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &totp, &disabled, &must,
		&last, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.TOTPEnabled, u.Disabled, u.MustChangePassword = totp == 1, disabled == 1, must == 1
	if last.Valid {
		v := last.String
		u.LastLoginAt = &v
	}
	return &u, nil
}

// UpdateUserFields carries optional changes; nil fields are left untouched.
type UpdateUserFields struct {
	Role        *string
	DisplayName *string
	Disabled    *bool
}

// UpdateUser applies partial updates.
func (s *Service) UpdateUser(ctx context.Context, id string, f UpdateUserFields) error {
	if f.Role != nil {
		if !ValidRole(*f.Role) {
			return fmt.Errorf("invalid role %q", *f.Role)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET role=?,updated_at=? WHERE id=?`,
			*f.Role, store.Now(), id); err != nil {
			return err
		}
	}
	if f.DisplayName != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET display_name=?,updated_at=? WHERE id=?`,
			*f.DisplayName, store.Now(), id); err != nil {
			return err
		}
	}
	if f.Disabled != nil {
		v := 0
		if *f.Disabled {
			v = 1
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET disabled=?,updated_at=? WHERE id=?`,
			v, store.Now(), id); err != nil {
			return err
		}
		if v == 1 { // revoke all sessions of a disabled account
			_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET revoked=1,revoke_reason='user_disabled' WHERE user_id=?`, id)
		}
	}
	return nil
}

// SetPassword validates and replaces a password hash.
func (s *Service) SetPassword(ctx context.Context, id, newPassword string) error {
	if probs := PasswordStrength(newPassword); len(probs) > 0 {
		return fmt.Errorf("weak password: %s", strings.Join(probs, ", "))
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET password_hash=?,must_change_password=0,updated_at=? WHERE id=?`,
		hash, store.Now(), id)
	return err
}

// DeleteUser removes a user. The caller must enforce the "last owner" rule.
func (s *Service) DeleteUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// CountOwners returns active owner count (last-owner protection).
func (s *Service) CountOwners(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE role='owner' AND disabled=0`).Scan(&n)
	return n, err
}

// SessionView is a session row safe to return to the API.
type SessionView struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
	LastSeen  string `json:"lastSeenAt"`
	UserAgent string `json:"userAgent"`
	IP        string `json:"ip"`
	Revoked   bool   `json:"revoked"`
	Current   bool   `json:"current,omitempty"`
}

// ListSessions returns sessions for a user.
func (s *Service) ListSessions(ctx context.Context, userID, currentDigest string) ([]SessionView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,created_at,expires_at,last_seen_at,
        COALESCE(user_agent,''),COALESCE(ip,''),revoked FROM sessions WHERE user_id=?
        ORDER BY last_seen_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionView
	for rows.Next() {
		var v SessionView
		var revoked int
		if err := rows.Scan(&v.ID, &v.UserID, &v.CreatedAt, &v.ExpiresAt, &v.LastSeen,
			&v.UserAgent, &v.IP, &revoked); err != nil {
			return nil, err
		}
		v.Revoked = revoked == 1
		v.Current = v.ID == currentDigest
		out = append(out, v)
	}
	return out, rows.Err()
}

// BeginTOTPEnroll generates a secret, seals it and stores it in a
// not-yet-enabled state, returning secret + provisioning URI.
func (s *Service) BeginTOTPEnroll(ctx context.Context, userID string) (secret, uri string, err error) {
	if s.vault == nil {
		return "", "", errors.New("vault unavailable")
	}
	secret, err = GenerateTOTPSecret()
	if err != nil {
		return "", "", err
	}
	enc, err := s.vault.SealString(secret, "totp:"+userID)
	if err != nil {
		return "", "", err
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE users SET totp_secret_enc=?,updated_at=? WHERE id=?`,
		enc, store.Now(), userID); err != nil {
		return "", "", err
	}
	return secret, ProvisioningURI("BastionDeck", userID, secret), nil
}

// EnableTOTP validates a code against the staged secret and flips totp_enabled.
func (s *Service) EnableTOTP(ctx context.Context, userID, code string) error {
	row := s.db.QueryRowContext(ctx, `SELECT totp_secret_enc FROM users WHERE id=?`, userID)
	var enc []byte
	if err := row.Scan(&enc); err != nil {
		return err
	}
	secret, err := s.vault.Open(enc, "totp:"+userID)
	if err != nil {
		return err
	}
	if !ValidateTOTP(string(secret), code, timeNow()) {
		return errors.New("invalid TOTP code")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET totp_enabled=1,updated_at=? WHERE id=?`, store.Now(), userID)
	return err
}

// DisableTOTP turns off 2FA (requires a valid recent code or owner override).
func (s *Service) DisableTOTP(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_enabled=0,totp_secret_enc=NULL,updated_at=? WHERE id=?`, store.Now(), userID)
	return err
}
