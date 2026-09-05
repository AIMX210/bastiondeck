package snippets

import (
	"context"
	"strings"
	"testing"

	"bastiondeck/internal/testutil"
)

func TestRequiredVarsSortedUnique(t *testing.T) {
	got := RequiredVars("echo ${name} ${zone} ${name}; ls ${zone}")
	want := []string{"name", "zone"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("vars = %v, want %v", got, want)
	}
	if len(RequiredVars("plain command")) != 0 {
		t.Fatal("no vars expected")
	}
}

func TestRenderMissingKeptAndReported(t *testing.T) {
	out, missing := Render("hi ${who} on ${host}", map[string]string{"who": "ops"})
	if out != "hi ops on ${host}" {
		t.Fatalf("rendered = %q", out)
	}
	if len(missing) != 1 || missing[0] != "host" {
		t.Fatalf("missing = %v", missing)
	}
	full, none := Render("a=${a}", map[string]string{"a": "1"})
	if full != "a=1" || len(none) != 0 {
		t.Fatalf("full=%q missing=%v", full, none)
	}
}

func TestSnippetLifecycle(t *testing.T) {
	h := testutil.NewHarness(t)
	s := New(h.Store.DB)
	ctx := context.Background()

	created, err := s.Create(ctx, "Disk usage", "df -h ${mount}", []string{"fs", "read"}, "usr_1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ID, "snp_") {
		t.Fatalf("id prefix wrong: %s", created.ID)
	}
	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Disk usage" || len(got.Tags) != 2 {
		t.Fatalf("get = %+v", got)
	}

	list, err := s.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d, err %v", len(list), err)
	}

	upd, err := s.Update(ctx, created.ID, "Disk usage v2", "df -hT ${mount}", []string{"fs"})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Title != "Disk usage v2" || len(upd.Tags) != 1 {
		t.Fatalf("update = %+v", upd)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.List(ctx)
	if len(list) != 0 {
		t.Fatalf("after delete = %d", len(list))
	}
	if _, err := s.Get(ctx, created.ID); err == nil {
		t.Fatal("expected error fetching deleted snippet")
	}
}

func TestCreateRejectsEmpty(t *testing.T) {
	h := testutil.NewHarness(t)
	s := New(h.Store.DB)
	if _, err := s.Create(context.Background(), "", "body", nil, ""); err == nil {
		t.Fatal("empty title must be rejected")
	}
	if _, err := s.Create(context.Background(), "t", "", nil, ""); err == nil {
		t.Fatal("empty body must be rejected")
	}
}
