// Package vault implements the encrypted credential vault. Secrets are
// sealed with AES-256-GCM under a 32-byte master key; the Additional
// Authenticated Data binds a ciphertext to its record id so a blob cannot be
// moved between rows unnoticed.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrInvalidCiphertext is returned when a sealed blob is malformed or fails
// authentication.
var ErrInvalidCiphertext = errors.New("vault: ciphertext invalid or authentication failed")

// Vault holds the unwrapped master key in process memory only.
type Vault struct {
	key    []byte
	source string
}

// Load resolves the master key: explicit hex key wins, otherwise a 0600 key
// file under dataDir is read (and created on first run).
func Load(hexKey, dataDir string) (*Vault, error) {
	if hexKey != "" {
		k, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, fmt.Errorf("master key hex: %w", err)
		}
		if len(k) != 32 {
			return nil, errors.New("master key must decode to 32 bytes")
		}
		return &Vault{key: k, source: "env"}, nil
	}
	keyPath := filepath.Join(dataDir, "master.key")
	if raw, err := os.ReadFile(keyPath); err == nil {
		k, err := hex.DecodeString(string(raw))
		if err != nil || len(k) != 32 {
			return nil, errors.New("master.key corrupt: expected 64 hex chars")
		}
		return &Vault{key: k, source: "file"}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read master.key: %w", err)
	}
	// First run: generate and persist with restrictive permissions.
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(k)), 0o600); err != nil {
		return nil, fmt.Errorf("write master.key: %w", err)
	}
	return &Vault{key: k, source: "generated"}, nil
}

// FromHex builds a vault directly from a 64-hex-char key without touching the
// key file (used by key rotation through the local control plane).
func FromHex(hexKey string) (*Vault, error) {
	k, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("master key hex: %w", err)
	}
	if len(k) != 32 {
		return nil, errors.New("master key must decode to 32 bytes")
	}
	return &Vault{key: k, source: "rotated"}, nil
}

// Source reports where the key came from (env/file/generated), for doctor.
func (v *Vault) Source() string { return v.source }

// Seal encrypts plaintext, binding it to aad (typically the record id).
// Layout: nonce(12) || gcm.Seal(nil, nonce, plaintext, aad).
func (v *Vault) Seal(plaintext []byte, aad string) ([]byte, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, []byte(aad))
	return ct, nil
}

// Open decrypts a blob produced by Seal, verifying the aad binding.
func (v *Vault) Open(blob []byte, aad string) ([]byte, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(blob) < ns+1 {
		return nil, ErrInvalidCiphertext
	}
	nonce, ct := blob[:ns], blob[ns:]
	pt, err := gcm.Open(nil, nonce, ct, []byte(aad))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}
	return pt, nil
}

// SealString is a convenience wrapper.
func (v *Vault) SealString(s, aad string) ([]byte, error) {
	return v.Seal([]byte(s), aad)
}

// OpenString is a convenience wrapper.
func (v *Vault) OpenString(blob []byte, aad string) (string, error) {
	b, err := v.Open(blob, aad)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Reseal decrypts under the current vault and re-encrypts under dst (used by
// master-key rotation flows). The AAD is preserved.
func (v *Vault) Reseal(blob []byte, aad string, dst *Vault) ([]byte, error) {
	pt, err := v.Open(blob, aad)
	if err != nil {
		return nil, err
	}
	return dst.Seal(pt, aad)
}
