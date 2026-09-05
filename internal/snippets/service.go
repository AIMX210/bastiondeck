// Package snippets stores reusable command snippets and renders their
// variables. Rendering is a pure substitution: snippets never auto-execute,
// and multi-line bodies are shown in a preview before running.
package snippets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	"bastiondeck/internal/store"
)

// Snippet is a reusable command template.
type Snippet struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
	CreatedBy string   `json:"createdBy,omitempty"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

// Service persists snippets.
type Service struct{ db *sql.DB }

// New constructs the service.
func New(db *sql.DB) *Service { return &Service{db: db} }

// varRe tolerates optional whitespace inside the braces, e.g. "${ name }",
// matching what the web wizard accepts.
var varRe = regexp.MustCompile(`\$\{\s*([a-zA-Z0-9_.-]+)\s*\}`)

// RequiredVars returns the sorted unique variables referenced in a body.
func RequiredVars(body string) []string {
	found := map[string]bool{}
	for _, m := range varRe.FindAllStringSubmatch(body, -1) {
		found[m[1]] = true
	}
	out := make([]string, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Render substitutes ${var} occurrences. Missing variables are kept intact
// and reported so the UI can prompt rather than silently sending a template.
func Render(body string, vars map[string]string) (string, []string) {
	var missing []string
	out := varRe.ReplaceAllStringFunc(body, func(m string) string {
		name := varRe.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		missing = append(missing, name)
		return m
	})
	return out, missing
}

// Create inserts a snippet.
func (s *Service) Create(ctx context.Context, title, body string, tags []string, by string) (*Snippet, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title required")
	}
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("body required")
	}
	if tags == nil {
		tags = []string{}
	}
	tb, _ := json.Marshal(tags)
	sn := &Snippet{ID: store.NewID(store.PrefixSnippet), Title: title, Body: body, Tags: tags,
		CreatedBy: by, CreatedAt: store.Now(), UpdatedAt: store.Now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO snippets(id,title,body,tags,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		sn.ID, sn.Title, sn.Body, string(tb), by, sn.CreatedAt, sn.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return sn, nil
}

// List returns all snippets.
func (s *Service) List(ctx context.Context) ([]Snippet, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,title,body,tags,COALESCE(created_by,''),created_at,updated_at FROM snippets ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snippet
	for rows.Next() {
		sn, err := scanSnippet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sn)
	}
	return out, rows.Err()
}

// Get loads one snippet.
func (s *Service) Get(ctx context.Context, id string) (*Snippet, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,title,body,tags,COALESCE(created_by,''),created_at,updated_at FROM snippets WHERE id=?`, id)
	sn, err := scanSnippet(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return sn, err
}

// Update changes a snippet.
func (s *Service) Update(ctx context.Context, id, title, body string, tags []string) (*Snippet, error) {
	if tags == nil {
		tags = []string{}
	}
	tb, _ := json.Marshal(tags)
	res, err := s.db.ExecContext(ctx,
		`UPDATE snippets SET title=?,body=?,tags=?,updated_at=? WHERE id=?`,
		title, body, string(tb), store.Now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, store.ErrNotFound
	}
	return s.Get(ctx, id)
}

// Delete removes a snippet.
func (s *Service) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM snippets WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scanSnippet(sc scanner) (*Snippet, error) {
	var sn Snippet
	var tags string
	if err := sc.Scan(&sn.ID, &sn.Title, &sn.Body, &tags, &sn.CreatedBy, &sn.CreatedAt, &sn.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tags), &sn.Tags)
	if sn.Tags == nil {
		sn.Tags = []string{}
	}
	return &sn, nil
}
