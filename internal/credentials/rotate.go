package credentials

import (
	"context"

	"bastiondeck/internal/vault"
)

// RotateVault re-seals every credential ciphertext from the current master
// key to dst inside one transaction, then swaps the active vault. The AAD
// (credential id) is preserved across re-seal.
func (s *Service) RotateVault(ctx context.Context, dst *vault.Vault) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,ciphertext FROM credentials`)
	if err != nil {
		return 0, err
	}
	type pair struct {
		id   string
		blob []byte
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.blob); err != nil {
			rows.Close()
			return 0, err
		}
		pairs = append(pairs, p)
	}
	rows.Close()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, p := range pairs {
		plain, err := s.vault.OpenString(p.blob, p.id)
		if err != nil {
			return 0, err
		}
		resealed, err := dst.SealString(plain, p.id)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE credentials SET ciphertext=? WHERE id=?`, resealed, p.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.vault = dst
	return len(pairs), nil
}
