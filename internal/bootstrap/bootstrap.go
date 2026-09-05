// Package bootstrap enforces the first-run setup gate: until an owner exists
// only /api/status and /api/setup are reachable, preventing an uninitialised
// internet-facing instance from being claimed by a scanner.
package bootstrap

import (
	"context"
	"errors"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/auth"
)

// Service coordinates first-run owner creation.
type Service struct {
	users *auth.Service
	logs  *audit.Service
}

// New constructs the bootstrap service.
func New(users *auth.Service, logs *audit.Service) *Service {
	return &Service{users: users, logs: logs}
}

// SetupRequired reports whether no user exists yet.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	n, err := s.users.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// SetupInput creates the first owner.
type SetupInput struct {
	Username    string
	Password    string
	DisplayName string
}

// ErrAlreadySetup is returned when setup runs against an initialised system.
var ErrAlreadySetup = errors.New("setup already completed")

// InitialSetup creates the owner exactly once and writes an audit trail.
func (s *Service) InitialSetup(ctx context.Context, in SetupInput, ip string) (*auth.User, error) {
	required, err := s.SetupRequired(ctx)
	if err != nil {
		return nil, err
	}
	if !required {
		return nil, ErrAlreadySetup
	}
	u, err := s.users.CreateUser(ctx, auth.CreateUserInput{
		Username:    in.Username,
		Password:    in.Password,
		DisplayName: in.DisplayName,
		Role:        auth.RoleOwner,
	})
	if err != nil {
		return nil, err
	}
	_, _ = s.logs.Write(ctx, audit.Actor{ID: u.ID, Name: u.Username, IP: ip},
		"setup.complete", "user", u.ID, "success", map[string]any{"role": u.Role})
	return u, nil
}
