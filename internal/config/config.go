// Package config loads runtime configuration from environment variables and
// command line flags with safe, loopback-first defaults.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config is the resolved runtime configuration. Every field is concrete and
// validated; downstream packages must not re-read the environment.
type Config struct {
	Listen       string        // TCP listen address for the web API
	DataDir      string        // writable directory for db, logs, artifacts
	MasterKeyHex string        // optional 64-hex-char master key
	ControlSock  string        // unix socket for the local control plane
	SessionTTL   time.Duration // sliding web session lifetime
	TrustProxy   bool          // honour X-Forwarded-* (only behind a reverse proxy)
	LogLevel     string        // debug|info|warn|error
	StaticDir    string        // optional disk override for embedded web assets (dev)
	EnableAgent  bool          // listen for bd-agent reverse connections
	AgentListen  string        // agent endpoint address (defaults to Listen)
	MaxOutputB   int64         // per-target captured output cap in bytes
	IdleConns    time.Duration // pooled SSH idle timeout
}

// Defaults returns a config populated with safe defaults; callers then apply
// environment overrides with FromEnv.
func Defaults() *Config {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	dataDir := filepath.Join(home, ".bastiondeck")
	sock := filepath.Join(dataDir, "control.sock")
	if rt := os.Getenv("XDG_RUNTIME_DIR"); runtime.GOOS != "darwin" && rt != "" {
		if err := os.MkdirAll(filepath.Join(rt, "bastiondeck"), 0o700); err == nil {
			sock = filepath.Join(rt, "bastiondeck", "control.sock")
		}
	}
	return &Config{
		Listen:      "127.0.0.1:8840",
		DataDir:     dataDir,
		ControlSock: sock,
		SessionTTL:  12 * time.Hour,
		LogLevel:    "info",
		EnableAgent: true,
		MaxOutputB:  4 << 20,
		IdleConns:   5 * time.Minute,
	}
}

// FromEnv overlays BDK_* environment variables onto the receiver.
func (c *Config) FromEnv() error {
	var problems []string
	get := func(k string) (string, bool) {
		v, ok := os.LookupEnv("BDK_" + k)
		return strings.TrimSpace(v), ok
	}
	if v, ok := get("LISTEN"); ok {
		c.Listen = v
	}
	if v, ok := get("DATA_DIR"); ok && v != "" {
		c.DataDir = v
	}
	if v, ok := get("MASTER_KEY"); ok {
		c.MasterKeyHex = v
	}
	if v, ok := get("CONTROL_SOCK"); ok && v != "" {
		c.ControlSock = v
	}
	if v, ok := get("STATIC_DIR"); ok {
		c.StaticDir = v
	}
	if v, ok := get("LOG_LEVEL"); ok && v != "" {
		c.LogLevel = strings.ToLower(v)
	}
	if v, ok := get("TRUST_PROXY"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			problems = append(problems, "BDK_TRUST_PROXY must be a boolean")
		} else {
			c.TrustProxy = b
		}
	}
	if v, ok := get("ENABLE_AGENT"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			problems = append(problems, "BDK_ENABLE_AGENT must be a boolean")
		} else {
			c.EnableAgent = b
		}
	}
	if v, ok := get("AGENT_LISTEN"); ok && v != "" {
		c.AgentListen = v
	}
	if v, ok := get("SESSION_TTL"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			problems = append(problems, "BDK_SESSION_TTL must be a positive duration like 12h")
		} else {
			c.SessionTTL = d
		}
	}
	if v, ok := get("MAX_OUTPUT_BYTES"); ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			problems = append(problems, "BDK_MAX_OUTPUT_BYTES must be a positive integer")
		} else {
			c.MaxOutputB = n
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Validate checks invariants that cannot be enforced while parsing.
func (c *Config) Validate() error {
	if c.DataDir == "" {
		return errors.New("data dir is required")
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("invalid listen address %q: %w", c.Listen, err)
	}
	if c.MasterKeyHex != "" && len(c.MasterKeyHex) != 64 {
		return errors.New("BDK_MASTER_KEY must be 64 hex characters (32 bytes)")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level %q", c.LogLevel)
	}
	if c.SessionTTL < time.Minute {
		return errors.New("session TTL must be at least 1 minute")
	}
	return nil
}

// Load builds config from defaults and environment and ensures the data
// directory exists with 0700 permissions.
func Load() (*Config, error) {
	c := Defaults()
	if err := c.FromEnv(); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	for _, sub := range []string{"runs", "transfers", "backups", "tmp"} {
		if err := os.MkdirAll(filepath.Join(c.DataDir, sub), 0o700); err != nil {
			return nil, fmt.Errorf("create %s dir: %w", sub, err)
		}
	}
	return c, nil
}

// ArtifactDir returns the absolute path for a class of on-disk artifacts.
func (c *Config) ArtifactDir(kind string) string {
	return filepath.Join(c.DataDir, kind)
}
