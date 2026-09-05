package credentials

import (
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// storedPayload is the on-disk plaintext shape sealed by the vault.
type storedPayload struct {
	Kind       string `json:"kind"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase,omitempty"`
}

func encodeSecret(kind, secret, passphrase string) (string, error) {
	b, err := json.Marshal(storedPayload{Kind: kind, Secret: secret, Passphrase: passphrase})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeSecret(kind, payload string) (*Secret, error) {
	var p storedPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return nil, err
	}
	if p.Kind != kind {
		return nil, errors.New("credential kind mismatch")
	}
	return &Secret{Kind: p.Kind, Password: p.Secret, PrivatePEM: p.Secret, Passphrase: p.Passphrase}, nil
}

// AuthMethod converts a revealed secret into an ssh.AuthMethod.
func (s *Secret) AuthMethod() (ssh.AuthMethod, error) {
	switch s.Kind {
	case KindPassword:
		return ssh.Password(s.Password), nil
	case KindPrivateKey:
		pem := []byte(s.PrivatePEM)
		if s.Passphrase != "" {
			signer, err := ssh.ParsePrivateKeyWithPassphrase(pem, []byte(s.Passphrase))
			if err != nil {
				return nil, fmt.Errorf("parse encrypted private key: %w", err)
			}
			return ssh.PublicKeys(signer), nil
		}
		signer, err := ssh.ParsePrivateKey(pem)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, errors.New("unknown credential kind")
	}
}

// PublicFingerprint computes the SHA256 fingerprint of a private key's public
// half for display, e.g. SHA256:xxxx.
func PublicFingerprint(pem string) (string, error) {
	signer, err := ssh.ParsePrivateKey([]byte(pem))
	if err != nil {
		return "", err
	}
	return ssh.FingerprintSHA256(signer.PublicKey()), nil
}
