// Package audit provides the tamper-evident audit log. Every row stores the
// SHA-256 hash of its canonical payload chained to the previous row's hash,
// so deletion or edit of any historical record is detectable by Verify.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"bastiondeck/internal/store"
)

// Entry is one audit record.
type Entry struct {
	EventID    string          `json:"eventId"`
	At         string          `json:"at"`
	ActorID    string          `json:"actorId,omitempty"`
	ActorName  string          `json:"actorName,omitempty"`
	Action     string          `json:"action"`
	ObjectType string          `json:"objectType,omitempty"`
	ObjectID   string          `json:"objectId,omitempty"`
	Result     string          `json:"result"` // success|denied|failure
	Detail     json.RawMessage `json:"detail,omitempty"`
	PrevHash   string          `json:"prevHash,omitempty"`
	Hash       string          `json:"hash"`
	IP         string          `json:"ip,omitempty"`
	Seq        int64           `json:"seq"`
}

// Actor describes who performed an action.
type Actor struct {
	ID   string
	Name string
	IP   string
}

// Service writes and queries the audit chain.
type Service struct {
	db *sql.DB
}

// New constructs the audit service.
func New(db *sql.DB) *Service { return &Service{db: db} }

// canonicalPayload deterministically serialises the hashed fields. Go's
// encoding/json sorts map keys, giving stable output.
func canonicalPayload(at, eventID, actorID, actorName, action, objType, objID, result, prevHash string, detail json.RawMessage) string {
	if len(detail) == 0 {
		detail = json.RawMessage("{}")
	}
	m := map[string]any{
		"at": at, "eventId": eventID, "actorId": actorID, "actorName": actorName,
		"action": action, "objectType": objType, "objectId": objID,
		"result": result, "prevHash": prevHash, "detail": detail,
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// Write appends a chained entry. Detail may be nil or any JSON-serialisable
// value. This never returns an error to callers that cannot act on it? It
// does: audit failure must be visible to the caller.
func (s *Service) Write(ctx context.Context, actor Actor, action, objType, objID, result string, detail any) (*Entry, error) {
	var rawDetail json.RawMessage
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return nil, fmt.Errorf("audit detail: %w", err)
		}
		rawDetail = b
	}
	at := store.Now()
	eventID := store.NewID(store.PrefixAudit)

	// 哈希链必须串行化：读取 prevHash 与 INSERT 必须在同一事务内完成，
	// 否则并发写入会读到同一条 prevHash 导致链分叉，Verify 误报篡改。
	var entry *Entry
	txErr := store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var prevHash string
		var seq int64
		err := tx.QueryRow(`SELECT COALESCE(seq,0), COALESCE(hash,'') FROM (
            SELECT id AS seq, hash FROM audit_logs ORDER BY id DESC LIMIT 1)`).Scan(&seq, &prevHash)
		if errors.Is(err, sql.ErrNoRows) {
			prevHash, seq = "", 0
		} else if err != nil {
			return err
		}
		payload := canonicalPayload(at, eventID, actor.ID, actor.Name, action, objType, objID, result, prevHash, rawDetail)
		hash := store.HashToken(payload)

		if _, err := tx.Exec(`INSERT INTO audit_logs
            (event_id,at,actor_id,actor_name,action,object_type,object_id,result,detail_json,prev_hash,hash,ip)
            VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			eventID, at, nullStr(actor.ID), nullStr(actor.Name), action, nullStr(objType), nullStr(objID),
			result, string(orEmpty(rawDetail)), prevHash, hash, nullStr(actor.IP)); err != nil {
			return err
		}
		entry = &Entry{
			EventID: eventID, At: at, ActorID: actor.ID, ActorName: actor.Name,
			Action: action, ObjectType: objType, ObjectID: objID, Result: result,
			Detail: rawDetail, PrevHash: prevHash, Hash: hash, IP: actor.IP, Seq: seq + 1,
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return entry, nil
}

func orEmpty(b json.RawMessage) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ListPage returns entries ordered newest-first with opaque cursor pagination.
func (s *Service) ListPage(ctx context.Context, limit int, cursor int64, filter Filter) ([]Entry, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := "WHERE 1=1"
	args := []any{}
	if filter.Actor != "" {
		where += " AND actor_name LIKE ?"
		args = append(args, "%"+filter.Actor+"%")
	}
	if filter.Action != "" {
		where += " AND action LIKE ?"
		args = append(args, "%"+filter.Action+"%")
	}
	if filter.Result != "" {
		where += " AND result = ?"
		args = append(args, filter.Result)
	}
	if filter.From != "" {
		where += " AND at >= ?"
		args = append(args, filter.From)
	}
	if filter.To != "" {
		where += " AND at <= ?"
		args = append(args, filter.To)
	}
	if cursor > 0 {
		// cursor is the id of the first unreturned row (the extra row probed
		// on the previous page), so it must be included here; strict less-than
		// would drop one entry per page.
		where += " AND id <= ?"
		args = append(args, cursor)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT id,event_id,at,COALESCE(actor_id,''),COALESCE(actor_name,''),
        action,COALESCE(object_type,''),COALESCE(object_id,''),result,detail_json,prev_hash,hash,COALESCE(ip,'')
        FROM audit_logs `+where+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Entry
	var next int64
	for rows.Next() {
		var e Entry
		var detail string
		if err := rows.Scan(&e.Seq, &e.EventID, &e.At, &e.ActorID, &e.ActorName, &e.Action,
			&e.ObjectType, &e.ObjectID, &e.Result, &detail, &e.PrevHash, &e.Hash, &e.IP); err != nil {
			return nil, 0, err
		}
		e.Detail = json.RawMessage(detail)
		if len(out) == limit {
			next = e.Seq
			break
		}
		out = append(out, e)
	}
	return out, next, rows.Err()
}

// Filter narrows an audit listing.
type Filter struct {
	Actor, Action, Result, From, To string
}

// ChainReport is the result of a full chain verification.
type ChainReport struct {
	OK       bool   `json:"ok"`
	Checked  int    `json:"checked"`
	BrokenAt int64  `json:"brokenAt,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Verify recomputes every hash in insertion order and reports the first break.
func (s *Service) Verify(ctx context.Context) (ChainReport, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,event_id,at,COALESCE(actor_id,''),COALESCE(actor_name,''),
        action,COALESCE(object_type,''),COALESCE(object_id,''),result,detail_json,prev_hash,hash
        FROM audit_logs ORDER BY id ASC`)
	if err != nil {
		return ChainReport{}, err
	}
	defer rows.Close()
	prev := ""
	n := 0
	for rows.Next() {
		var seq int64
		var e Entry
		var detail string
		if err := rows.Scan(&seq, &e.EventID, &e.At, &e.ActorID, &e.ActorName, &e.Action,
			&e.ObjectType, &e.ObjectID, &e.Result, &detail, &e.PrevHash, &e.Hash); err != nil {
			return ChainReport{}, err
		}
		if e.PrevHash != prev {
			return ChainReport{OK: false, Checked: n, BrokenAt: seq, Reason: "prev_hash mismatch"}, nil
		}
		want := store.HashToken(canonicalPayload(e.At, e.EventID, e.ActorID, e.ActorName,
			e.Action, e.ObjectType, e.ObjectID, e.Result, prev, json.RawMessage(detail)))
		if want != e.Hash {
			return ChainReport{OK: false, Checked: n, BrokenAt: seq, Reason: "payload hash mismatch"}, nil
		}
		prev = e.Hash
		n++
	}
	return ChainReport{OK: true, Checked: n}, rows.Err()
}

// Count returns total audit rows (for doctor/export).
func (s *Service) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&n)
	return n, err
}
