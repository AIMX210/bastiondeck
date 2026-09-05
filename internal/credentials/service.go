// Package credentials is the business layer over the encrypted credential
// vault: list/create/update/delete without ever returning ciphertext, and a
// single Reveal path used only while establishing a connection.
package credentials

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"bastiondeck/internal/store"
	"bastiondeck/internal/vault"
)

// Kind constants.
const (
	KindPassword   = "password"
	KindPrivateKey = "private_key"
)

// Credential is the safe projection returned by the API (no secret material).
type Credential struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Fingerprint string `json:"fingerprint,omitempty"`
	CreatedBy   string `json:"createdBy"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// Secret is the decrypted material used only at connect time, never logged.
type Secret struct {
	Kind string
	// Password holds the password for KindPassword; for private keys it is
	// unused unless the key is encrypted, in which case Passphrase applies.
	Password   string
	PrivatePEM string
	Passphrase string
}

// Service manages credentials.
type Service struct {
	db    *sql.DB
	vault *vault.Vault
}

// New constructs the service.
func New(db *sql.DB, v *vault.Vault) *Service { return &Service{db: db, vault: v} }

// Create seals and stores a credential. secret is password text or PEM key.
func (s *Service) Create(ctx context.Context, name, kind, secret, passphrase, createdBy string) (*Credential, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("credential name required")
	}
	if kind != KindPassword && kind != KindPrivateKey {
		return nil, errors.New("kind must be password or private_key")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("secret is empty")
	}
	id := store.NewID(store.PrefixCred)
	payload, err := encodeSecret(kind, secret, passphrase)
	if err != nil {
		return nil, err
	}
	blob, err := s.vault.SealString(payload, id)
	if err != nil {
		return nil, err
	}
	fp := ""
	if kind == KindPrivateKey {
		fp, err = PublicFingerprint(secret)
		if err != nil {
			return nil, fmt.Errorf("private key: %w", err)
		}
	}
	c := &Credential{ID: id, Name: name, Kind: kind, Fingerprint: fp, CreatedBy: createdBy,
		CreatedAt: store.Now(), UpdatedAt: store.Now()}
	_, err = s.db.ExecContext(ctx, `INSERT INTO credentials
        (id,name,kind,ciphertext,fingerprint,created_by,created_at,updated_at)
        VALUES(?,?,?,?,?,?,?,?)`, c.ID, c.Name, c.Kind, blob, fp, createdBy, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// List returns safe projections.
func (s *Service) List(ctx context.Context) ([]Credential, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,name,kind,COALESCE(fingerprint,''),created_by,created_at,updated_at
         FROM credentials ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Fingerprint, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns a safe projection by id.
func (s *Service) Get(ctx context.Context, id string) (*Credential, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,name,kind,COALESCE(fingerprint,''),created_by,created_at,updated_at
         FROM credentials WHERE id=?`, id)
	var c Credential
	err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.Fingerprint, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Update changes name and/or rotates the sealed secret.
func (s *Service) Update(ctx context.Context, id, name string, secret, passphrase *string) (*Credential, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != "" {
		c.Name = strings.TrimSpace(name)
	}
	if secret != nil && strings.TrimSpace(*secret) != "" {
		payload, err := encodeSecret(c.Kind, *secret, passphraseOrEmpty(passphrase))
		if err != nil {
			return nil, err
		}
		blob, err := s.vault.SealString(payload, id)
		if err != nil {
			return nil, err
		}
		fp := c.Fingerprint
		if c.Kind == KindPrivateKey {
			fp, _ = PublicFingerprint(*secret)
		}
		_, err = s.db.ExecContext(ctx,
			`UPDATE credentials SET name=?,ciphertext=?,fingerprint=?,updated_at=? WHERE id=?`,
			c.Name, blob, fp, store.Now(), id)
		if err != nil {
			return nil, err
		}
		c.Fingerprint = fp
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE credentials SET name=?,updated_at=? WHERE id=?`,
			c.Name, store.Now(), id)
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

// InUse reports hosts referencing the credential.
func (s *Service) InUse(ctx context.Context, id string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hosts WHERE credential_id=?`, id).Scan(&n)
	return n, err
}

// Delete removes a credential; it must not be in use.
func (s *Service) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Reveal decrypts a credential. This is the ONLY path that returns plaintext
// and must be called from connection establishment code paths only.
func (s *Service) Reveal(ctx context.Context, id string) (*Secret, error) {
	var kind string
	var blob []byte
	err := s.db.QueryRowContext(ctx, `SELECT kind,ciphertext FROM credentials WHERE id=?`, id).
		Scan(&kind, &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	payload, err := s.vault.Open(blob, id)
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	return decodeSecret(kind, string(payload))
}

func passphraseOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
