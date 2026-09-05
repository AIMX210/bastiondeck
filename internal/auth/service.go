package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"bastiondeck/internal/store"
	"bastiondeck/internal/vault"
)

// Service handles user and session lifecycle.
type Service struct {
	db    *sql.DB
	vault *vault.Vault
	ttl   time.Duration
}

// NewService constructs the auth service.
func NewService(db *sql.DB, v *vault.Vault, sessionTTL time.Duration) *Service {
	return &Service{db: db, vault: v, ttl: sessionTTL}
}

// LoginOutcome is returned by Login.
type LoginOutcome struct {
	User         User
	SessionToken string
	SessionID    string
}

const (
	maxFailedWindow = 10 * time.Minute
	maxFailed       = 10
)

// Login verifies credentials (and TOTP when enabled), enforces the rate
// limiter and creates a sliding session.
func (s *Service) Login(ctx context.Context, username, password, totpCode, ip, ua string) (*LoginOutcome, error) {
	username = strings.TrimSpace(username)
	if locked, err := s.isLocked(ctx, username, ip); err != nil {
		return nil, err
	} else if locked {
		return nil, ErrLocked
	}
	u, hash, totpEnc, err := s.userWithSecrets(ctx, username)
	if err != nil {
		_ = s.recordAttempt(ctx, username, ip, false)
		return nil, ErrBadCredentials
	}
	if u.Disabled {
		_ = s.recordAttempt(ctx, username, ip, false)
		return nil, ErrBadCredentials
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil || !ok {
		_ = s.recordAttempt(ctx, username, ip, false)
		return nil, ErrBadCredentials
	}
	if u.TOTPEnabled {
		secret := ""
		if s.vault != nil && totpEnc != nil {
			dec, err := s.vault.Open(totpEnc, "totp:"+u.ID)
			if err != nil {
				return nil, fmt.Errorf("totp unavailable: %w", err)
			}
			secret = string(dec)
		}
		if !ValidateTOTP(secret, totpCode, time.Now()) {
			_ = s.recordAttempt(ctx, username, ip, false)
			return nil, ErrBadCredentials
		}
	}
	if err := s.recordAttempt(ctx, username, ip, true); err != nil {
		return nil, err
	}
	token, digest, err := s.createSession(ctx, u.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE users SET last_login_at=? WHERE id=?`, store.Now(), u.ID)
	return &LoginOutcome{User: *u, SessionToken: token, SessionID: digest}, nil
}

func (s *Service) userWithSecrets(ctx context.Context, username string) (*User, string, []byte, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,username,display_name,role,password_hash,totp_secret_enc,
        totp_enabled,disabled,must_change_password,last_login_at,created_at,updated_at
        FROM users WHERE username=?`, username)
	var u User
	var hash string
	var totpEnc []byte
	var disabled, totpOn, mustChange int
	var last sql.NullString
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &hash, &totpEnc,
		&totpOn, &disabled, &mustChange, &last, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil, ErrBadCredentials
	}
	if err != nil {
		return nil, "", nil, err
	}
	u.TOTPEnabled, u.Disabled, u.MustChangePassword = totpOn == 1, disabled == 1, mustChange == 1
	if last.Valid {
		v := last.String
		u.LastLoginAt = &v
	}
	return &u, hash, totpEnc, nil
}

func (s *Service) createSession(ctx context.Context, userID, ip, ua string) (token, digest string, err error) {
	token = store.RandomToken(32)
	digest = store.HashToken(token)
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO sessions
        (id,user_id,created_at,expires_at,last_seen_at,user_agent,ip)
        VALUES(?,?,?,?,?,?,?)`,
		digest, userID, now.Format(time.RFC3339Nano), now.Add(s.ttl).Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano), truncUA(ua), ip)
	if err != nil {
		return "", "", err
	}
	return token, digest, nil
}

// Resolve validates a session token, enforces expiry and performs a sliding
// renewal when less than half the TTL remains.
func (s *Service) Resolve(ctx context.Context, token string) (*User, *Session, error) {
	if token == "" {
		return nil, nil, errors.New("empty session token")
	}
	return s.resolveByDigest(ctx, store.HashToken(token))
}

// resolveByDigest loads session + user by stored digest.
func (s *Service) resolveByDigest(ctx context.Context, digest string) (*User, *Session, error) {
	// We store digest in sessions.id for compactness; look it up directly.
	var sess Session
	var sessID, userID, created, expires, lastSeen, ua, ip string
	var revoked int
	err := s.db.QueryRowContext(ctx, `SELECT id,user_id,created_at,expires_at,last_seen_at,
        COALESCE(user_agent,''),COALESCE(ip,''),revoked FROM sessions WHERE id=?`, digest).
		Scan(&sessID, &userID, &created, &expires, &lastSeen, &ua, &ip, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errors.New("session not found")
	}
	if err != nil {
		return nil, nil, err
	}
	if revoked == 1 {
		return nil, nil, errors.New("session revoked")
	}
	exp, _ := time.Parse(time.RFC3339Nano, expires)
	if time.Now().UTC().After(exp) {
		return nil, nil, errors.New("session expired")
	}
	u, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if u.Disabled {
		return nil, nil, errors.New("user disabled")
	}
	sess.ID, sess.UserID = sessID, userID
	sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	sess.ExpiresAt = exp
	sess.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen)
	sess.UserAgent, sess.IP, sess.Revoked = ua, ip, revoked == 1

	// Sliding renewal: extend when under half TTL remaining.
	if time.Until(exp) < s.ttl/2 {
		newExp := time.Now().UTC().Add(s.ttl)
		_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET expires_at=?,last_seen_at=? WHERE id=?`,
			newExp.Format(time.RFC3339Nano), store.Now(), digest)
		sess.ExpiresAt = newExp
	}
	return u, &sess, nil
}

// CreateSessionForUser issues a session without a password (used by the
// first-run setup right after owner creation).
func (s *Service) CreateSessionForUser(ctx context.Context, userID, ip, ua string) (string, error) {
	token, _, err := s.createSession(ctx, userID, ip, ua)
	return token, err
}

// Logout revokes the session behind the token.
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked=1,revoke_reason='logout' WHERE id=?`,
		store.HashToken(token))
	return err
}

// Revoke revokes a session by id.
func (s *Service) Revoke(ctx context.Context, id, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked=1,revoke_reason=? WHERE id=?`, reason, id)
	return err
}

// RevokeOthers revokes every session of a user except keepID.
func (s *Service) RevokeOthers(ctx context.Context, userID, keepDigest string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET revoked=1,revoke_reason='revoke_others' WHERE user_id=? AND id<>? AND revoked=0`,
		userID, keepDigest)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Service) recordAttempt(ctx context.Context, username, ip string, ok bool) error {
	v := 0
	if ok {
		v = 1
		// A successful authentication proves the principal; clear earlier
		// failures for both the username and this IP so a later typo streak
		// does not accumulate on top of an already-proven identity.
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM login_attempts WHERE ok=0 AND (username=? OR ip=?)`,
			username, ip); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO login_attempts(username,ip,ok,at) VALUES(?,?,?,?)`,
		username, ip, v, store.Now())
	return err
}

func (s *Service) isLocked(ctx context.Context, username, ip string) (bool, error) {
	since := time.Now().UTC().Add(-maxFailedWindow).Format(time.RFC3339Nano)
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM login_attempts WHERE ok=0 AND at>=? AND (username=? OR ip=?)`,
		since, username, ip).Scan(&n)
	return n >= maxFailed, err
}

// PruneAttempts removes attempt rows older than the window (called hourly).
func (s *Service) PruneAttempts(ctx context.Context) error {
	cut := time.Now().UTC().Add(-2 * maxFailedWindow).Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE at<?`, cut)
	return err
}

func truncUA(ua string) string {
	if len(ua) > 300 {
		return ua[:300]
	}
	return ua
}
