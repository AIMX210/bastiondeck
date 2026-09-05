package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"bastiondeck/internal/store"
)

// Repo persists hosts and groups.
type Repo struct {
	db *sql.DB
}

// NewRepo constructs the repository.
func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// HostInput carries create/update fields.
type HostInput struct {
	Name         string
	Address      string
	Port         int
	Username     string
	CredentialID string
	AuthKind     string
	AgentID      string
	JumpHostID   string
	GroupID      string
	Tags         []string
	Notes        string
	Favorite     bool
	Options      map[string]string
}

const hostCols = `id,name,address,port,username,credential_id,auth_kind,agent_id,jump_host_id,group_id,
 tags,notes,favorite,known_host_key,known_host_key_type,first_seen_at,last_connected_at,last_status,
 last_status_at,options_json,created_at,updated_at`

func scanHost(sc rowScanner) (*Host, error) {
	var h Host
	var cred, agent, jump, group, key, keyType, first, lastConn, statusAt sql.NullString
	var fav int
	var tagsJSON, optsJSON string
	err := sc.Scan(&h.ID, &h.Name, &h.Address, &h.Port, &h.Username, &cred, &h.AuthKind, &agent,
		&jump, &group, &tagsJSON, &h.Notes, &fav, &key, &keyType, &first, &lastConn,
		&h.LastStatus, &statusAt, &optsJSON, &h.CreatedAt, &h.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	h.CredentialID, h.AgentID, h.JumpHostID, h.GroupID = strPtr(cred), strPtr(agent), strPtr(jump), strPtr(group)
	h.KnownHostKey, h.KnownKeyType, h.FirstSeenAt, h.LastConnected, h.LastStatusAt =
		strPtr(key), strPtr(keyType), strPtr(first), strPtr(lastConn), strPtr(statusAt)
	h.Tags = parseTags(tagsJSON)
	h.Options = parseOptions(optsJSON)
	h.Favorite = fav == 1
	return &h, nil
}

type rowScanner interface{ Scan(dest ...any) error }

// Create inserts a host after validating the jump graph.
func (r *Repo) Create(ctx context.Context, in HostInput) (*Host, error) {
	if err := r.validateInput(in); err != nil {
		return nil, err
	}
	if in.JumpHostID != "" {
		if err := r.checkJumpChain(ctx, in.JumpHostID, nil); err != nil {
			return nil, err
		}
	}
	h := &Host{
		ID: store.NewID(store.PrefixHost), Name: in.Name, Address: in.Address, Port: in.Port,
		Username: in.Username, AuthKind: defaultStr(in.AuthKind, "credential"),
		Tags: orEmptyTags(in.Tags), Options: orEmptyOpts(in.Options), Notes: in.Notes,
		Favorite: in.Favorite, LastStatus: "unknown", CreatedAt: store.Now(), UpdatedAt: store.Now(),
	}
	h.CredentialID = optStr(in.CredentialID)
	h.AgentID = optStr(in.AgentID)
	h.JumpHostID = optStr(in.JumpHostID)
	h.GroupID = optStr(in.GroupID)
	fav := 0
	if h.Favorite {
		fav = 1
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO hosts
        (id,name,address,port,username,credential_id,auth_kind,agent_id,jump_host_id,group_id,
         tags,notes,favorite,last_status,options_json,created_at,updated_at)
        VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.Name, h.Address, h.Port, h.Username, nullStr(in.CredentialID), h.AuthKind,
		nullStr(in.AgentID), nullStr(in.JumpHostID), nullStr(in.GroupID), marshalTags(h.Tags),
		h.Notes, fav, h.LastStatus, marshalOptions(h.Options), h.CreatedAt, h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// Update applies changes to an existing host.
func (r *Repo) Update(ctx context.Context, id string, in HostInput) (*Host, error) {
	old, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.validateInput(in); err != nil {
		return nil, err
	}
	// A host cannot be its own jump parent; check the chain excluding self.
	if in.JumpHostID != "" {
		if in.JumpHostID == id {
			return nil, ErrJumpCycle
		}
		if err := r.checkJumpChain(ctx, in.JumpHostID, map[string]bool{id: true}); err != nil {
			return nil, err
		}
	}
	fav := 0
	if in.Favorite {
		fav = 1
	}
	_, err = r.db.ExecContext(ctx, `UPDATE hosts SET name=?,address=?,port=?,username=?,
        credential_id=?,auth_kind=?,agent_id=?,jump_host_id=?,group_id=?,tags=?,notes=?,favorite=?,
        options_json=?,updated_at=? WHERE id=?`,
		in.Name, in.Address, in.Port, in.Username, nullStr(in.CredentialID),
		defaultStr(in.AuthKind, "credential"), nullStr(in.AgentID), nullStr(in.JumpHostID),
		nullStr(in.GroupID), marshalTags(orEmptyTags(in.Tags)), in.Notes, fav,
		marshalOptions(orEmptyOpts(in.Options)), store.Now(), id)
	if err != nil {
		return nil, err
	}
	_ = old
	return r.Get(ctx, id)
}

// Get loads a host by id.
func (r *Repo) Get(ctx context.Context, id string) (*Host, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+hostCols+` FROM hosts WHERE id=?`, id)
	return scanHost(row)
}

// HostFilter narrows a list query.
type HostFilter struct {
	Query, Tag, GroupID, Status string
	FavoritesOnly               bool
}

// List returns hosts matching a filter, with case-insensitive search over
// name/address/notes/tags.
func (r *Repo) List(ctx context.Context, f HostFilter) ([]Host, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.Query != "" {
		where = append(where, "(LOWER(name) LIKE ? OR LOWER(address) LIKE ? OR LOWER(notes) LIKE ? OR LOWER(tags) LIKE ?)")
		q := "%" + strings.ToLower(f.Query) + "%"
		args = append(args, q, q, q, q)
	}
	if f.Tag != "" {
		where = append(where, "LOWER(tags) LIKE ?")
		args = append(args, "%\""+strings.ToLower(f.Tag)+"\"%")
	}
	if f.GroupID != "" {
		where = append(where, "group_id=?")
		args = append(args, f.GroupID)
	}
	if f.Status != "" {
		where = append(where, "last_status=?")
		args = append(args, f.Status)
	}
	if f.FavoritesOnly {
		where = append(where, "favorite=1")
	}
	q := `SELECT ` + hostCols + ` FROM hosts WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY favorite DESC, name ASC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Host
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

// Delete removes a host unless other hosts depend on it as a jump host.
func (r *Repo) Delete(ctx context.Context, id string) error {
	var dependents int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM hosts WHERE jump_host_id=?`, id).Scan(&dependents); err != nil {
		return err
	}
	if dependents > 0 {
		return fmt.Errorf("%w: %d host(s) depend on it", ErrIsJumpHost, dependents)
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM hosts WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStatus records the result of the latest connection attempt.
func (r *Repo) SetStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET last_status=?,last_status_at=?,last_connected_at=CASE WHEN ?='ok' THEN ? ELSE last_connected_at END WHERE id=?`,
		status, store.Now(), status, store.Now(), id)
	return err
}

// RecordHostKey performs TOFU bookkeeping: first sight stores the key, a
// changed key returns ErrKeyChanged; an equal key is a no-op success.
func (r *Repo) RecordHostKey(ctx context.Context, id, keyType, fingerprint string) error {
	h, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if h.KnownHostKey != nil && *h.KnownHostKey != fingerprint {
		return &KeyChangedError{Want: *h.KnownHostKey, Got: fingerprint}
	}
	if h.KnownHostKey != nil {
		return nil
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE hosts SET known_host_key=?,known_host_key_type=?,first_seen_at=?,updated_at=? WHERE id=?`,
		fingerprint, keyType, store.Now(), store.Now(), id)
	return err
}

// ResetHostKey clears a stored fingerprint (audited admin action).
func (r *Repo) ResetHostKey(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET known_host_key=NULL,known_host_key_type=NULL,first_seen_at=NULL,updated_at=? WHERE id=?`,
		store.Now(), id)
	return err
}

// KeyChangedError carries both fingerprints for explicit reset flows.
type KeyChangedError struct {
	Want string
	Got  string
}

func (e *KeyChangedError) Error() string {
	return fmt.Sprintf("host key changed: known=%s got=%s", e.Want, e.Got)
}

// checkJumpChain walks jump_host_id links ensuring no cycle and bounded depth.
// skip lets an update ignore the node being moved.
func (r *Repo) checkJumpChain(ctx context.Context, startID string, skip map[string]bool) error {
	seen := map[string]bool{}
	cur := startID
	depth := 0
	for cur != "" {
		if skip != nil && skip[cur] {
			return ErrJumpCycle
		}
		if seen[cur] {
			return ErrJumpCycle
		}
		seen[cur] = true
		depth++
		if depth > MaxJumpDepth {
			return ErrJumpTooDeep
		}
		var next sql.NullString
		err := r.db.QueryRowContext(ctx, `SELECT jump_host_id FROM hosts WHERE id=?`, cur).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("jump host %s does not exist", cur)
		}
		if err != nil {
			return err
		}
		cur = next.String
	}
	return nil
}

func (r *Repo) validateInput(in HostInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(in.Address) == "" {
		return errors.New("address is required")
	}
	if in.Port == 0 {
		in.Port = 22
	}
	if in.Port < 1 || in.Port > 65535 {
		return errors.New("port must be 1..65535")
	}
	if strings.TrimSpace(in.Username) == "" {
		return errors.New("username is required")
	}
	if in.AuthKind != "" && in.AuthKind != "credential" && in.AuthKind != "agent" {
		return ErrBadAuthKind
	}
	return nil
}

func defaultStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func orEmptyTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}
func orEmptyOpts(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
