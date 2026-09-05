package config_test

import (
	"strings"
	"testing"
	"time"

	"bastiondeck/internal/config"
)

func setEnv(t *testing.T, k, v string) {
	t.Helper()
	t.Setenv("BDK_"+k, v)
}

func TestDefaultsAreValid(t *testing.T) {
	c := config.Defaults()
	if err := c.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if c.Listen != "127.0.0.1:8840" {
		t.Fatalf("default listen = %q", c.Listen)
	}
	if !c.EnableAgent {
		t.Fatal("agent endpoint enabled by default")
	}
}

func TestFromEnvOverrides(t *testing.T) {
	setEnv(t, "LISTEN", "0.0.0.0:9000")
	setEnv(t, "DATA_DIR", "/var/lib/bdk")
	setEnv(t, "LOG_LEVEL", "DEBUG")
	setEnv(t, "TRUST_PROXY", "true")
	setEnv(t, "SESSION_TTL", "30m")
	setEnv(t, "MAX_OUTPUT_BYTES", "1048576")
	c := config.Defaults()
	if err := c.FromEnv(); err != nil {
		t.Fatal(err)
	}
	if c.Listen != "0.0.0.0:9000" || c.DataDir != "/var/lib/bdk" {
		t.Fatalf("override failed: %+v", c)
	}
	if c.LogLevel != "debug" || !c.TrustProxy {
		t.Fatalf("bool/case override failed: %+v", c)
	}
	if c.SessionTTL != 30*time.Minute || c.MaxOutputB != 1<<20 {
		t.Fatalf("numeric override failed: %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFromEnvRejectsBadValues(t *testing.T) {
	cases := map[string]string{
		"TRUST_PROXY":      "maybe",
		"SESSION_TTL":      "-5m",
		"MAX_OUTPUT_BYTES": "zero",
	}
	for k, v := range cases {
		t.Run(k, func(t *testing.T) {
			t.Setenv("BDK_"+k, v)
			c := config.Defaults()
			if err := c.FromEnv(); err == nil {
				t.Fatalf("%s=%s must be rejected", k, v)
			}
		})
	}
}

func TestValidateCatchesInvariants(t *testing.T) {
	bad := []struct {
		name string
		mut  func(c *config.Config)
		want string
	}{
		{"no data dir", func(c *config.Config) { c.DataDir = "" }, "data dir"},
		{"bad listen", func(c *config.Config) { c.Listen = "no-port" }, "listen"},
		{"short key", func(c *config.Config) { c.MasterKeyHex = "abc" }, "64 hex"},
		{"bad level", func(c *config.Config) { c.LogLevel = "trace" }, "log level"},
		{"short ttl", func(c *config.Config) { c.SessionTTL = time.Second }, "TTL"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			c := config.Defaults()
			tc.mut(c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}
