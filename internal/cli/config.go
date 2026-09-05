// Package cli implements the bdk command-line client: configuration storage,
// subcommand routing and table rendering on top of apiclient.
package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is the persisted CLI profile.
type Config struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
	User    string `json:"user"`
	Role    string `json:"role"`
}

// ConfigPath returns ~/.config/bastiondeck/config.json (honours XDG_CONFIG_HOME).
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			base = "."
		} else {
			base = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(base, "bastiondeck", "config.json")
}

// LoadConfig reads the persisted profile.
func LoadConfig() (*Config, error) {
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save persists the profile with 0600 permissions.
func (c *Config) Save() error {
	p := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

// Clear removes the stored profile.
func Clear() error {
	err := os.Remove(ConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
