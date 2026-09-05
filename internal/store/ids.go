package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// ID prefixes make object kinds visible in logs and API payloads.
const (
	PrefixUser     = "usr_"
	PrefixCred     = "crd_"
	PrefixHost     = "hst_"
	PrefixGroup    = "grp_"
	PrefixSnippet  = "snp_"
	PrefixJob      = "job_"
	PrefixRun      = "run_"
	PrefixTarget   = "tgt_"
	PrefixTunnel   = "tun_"
	PrefixTerm     = "trm_"
	PrefixAgent    = "agt_"
	PrefixAudit    = "aud_"
	PrefixSession  = "ses_"
	PrefixTransfer = "xfx_"
	PrefixEnroll   = "enr_"
	PrefixBackup   = "bak_"
)

// NewID returns a random identifier with the given prefix. 16 random bytes
// give 128 bits of collision resistance.
func NewID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is non-recoverable on a healthy runtime.
		panic("cannot read cryptographic randomness: " + err.Error())
	}
	return prefix + hex.EncodeToString(b)
}

// TokenBytes returns n raw random bytes.
func TokenBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

// RandomToken returns a URL-safe opaque token (hex by default).
func RandomToken(nBytes int) string {
	return hex.EncodeToString(TokenBytes(nBytes))
}

// HashToken stores only a SHA-256 digest of opaque tokens (sessions,
// enrollment secrets): plaintext never hits disk.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
