package inventory

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"bastiondeck/internal/store"
)

// CreateGroup inserts a host group.
func (r *Repo) CreateGroup(ctx context.Context, name, parentID string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("group name required")
	}
	g := &Group{ID: store.NewID(store.PrefixGroup), Name: name, CreatedAt: store.Now(), UpdatedAt: store.Now()}
	if parentID != "" {
		g.ParentID = &parentID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO host_groups(id,name,parent_id,created_at,updated_at) VALUES(?,?,?,?,?)`,
		g.ID, g.Name, parentID, g.CreatedAt, g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// ListGroups returns all groups ordered by name.
func (r *Repo) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,name,parent_id,created_at,updated_at FROM host_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var parent sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &parent, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		if parent.Valid {
			p := parent.String
			g.ParentID = &p
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// RenameGroup updates a group name.
func (r *Repo) RenameGroup(ctx context.Context, id, name string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE host_groups SET name=?,updated_at=? WHERE id=?`, strings.TrimSpace(name), store.Now(), id)
	return err
}

// DeleteGroup removes a group; child hosts are detached (group_id SET NULL
// via FK) but never deleted.
func (r *Repo) DeleteGroup(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE hosts SET group_id=NULL WHERE group_id=?`, id)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM host_groups WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchRecent records a host access for recents.
func (r *Repo) TouchRecent(ctx context.Context, hostID, userID, kind string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO host_recents(host_id,user_id,at,kind) VALUES(?,?,?,?)`,
		hostID, userID, store.Now(), kind)
	return err
}

// RecentHosts returns the user's recently accessed host ids.
func (r *Repo) RecentHosts(ctx context.Context, userID string, limit int) ([]string, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT host_id FROM host_recents WHERE user_id=? ORDER BY at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
