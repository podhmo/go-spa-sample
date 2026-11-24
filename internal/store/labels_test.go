package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestLabelCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// A workspace-scoped discussion label.
	id, err := s.CreateLabel(ctx, 1, KindLabelDiscussion, LabelInput{Name: "question", Color: "#123456"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	l, err := s.GetLabel(ctx, id)
	if err != nil {
		t.Fatalf("GetLabel: %v", err)
	}
	if l.Name != "question" || l.Kind != KindLabelDiscussion || l.WorkspaceID != 1 || l.Archived {
		t.Fatalf("label = %+v", l)
	}

	// Kind separates the pools: a discussion label is invisible to issues.
	dl, err := s.ListLabels(ctx, 1, KindLabelDiscussion)
	if err != nil {
		t.Fatalf("ListLabels discussion: %v", err)
	}
	if len(dl) != 1 || dl[0].Name != "question" {
		t.Fatalf("discussion labels = %+v", dl)
	}

	// Same name in the same scope and kind is a duplicate.
	if _, err := s.CreateLabel(ctx, 1, KindLabelDiscussion, LabelInput{Name: "question", Color: "#000000"}); !errors.Is(err, ErrDuplicateLabel) {
		t.Fatalf("duplicate create err = %v, want ErrDuplicateLabel", err)
	}
	// Same name in another kind or scope is fine.
	if _, err := s.CreateLabel(ctx, 1, KindLabelIssue, LabelInput{Name: "question", Color: "#000000"}); err != nil {
		t.Fatalf("same name other kind: %v", err)
	}
	ws2, err := s.CreateWorkspace(ctx, WorkspaceInput{Slug: "second"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if _, err := s.CreateLabel(ctx, ws2, KindLabelDiscussion, LabelInput{Name: "question", Color: "#000000"}); err != nil {
		t.Fatalf("same name other ws: %v", err)
	}

	if err := s.UpdateLabel(ctx, id, LabelInput{Name: "renamed", Color: "#abcdef"}); err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}
	l, _ = s.GetLabel(ctx, id)
	if l.Name != "renamed" || l.Color != "#abcdef" {
		t.Fatalf("after update: %+v", l)
	}

	if err := s.ArchiveLabel(ctx, id, true); err != nil {
		t.Fatalf("ArchiveLabel: %v", err)
	}
	dl, _ = s.ListLabels(ctx, 1, KindLabelDiscussion)
	if len(dl) != 0 {
		t.Fatalf("archived label still listed: %+v", dl)
	}
	al, _ := s.ListArchivedLabels(ctx, 1, KindLabelDiscussion)
	if len(al) != 1 || al[0].Name != "renamed" {
		t.Fatalf("archived labels = %+v", al)
	}

	// Re-creating an archived name restores the row with the new color.
	rid, err := s.CreateLabel(ctx, 1, KindLabelDiscussion, LabelInput{Name: "renamed", Color: "#111111"})
	if err != nil {
		t.Fatalf("re-create archived: %v", err)
	}
	if rid != id {
		t.Fatalf("restore id = %d, want %d", rid, id)
	}
	l, _ = s.GetLabel(ctx, id)
	if l.Archived || l.Color != "#111111" {
		t.Fatalf("restored label = %+v", l)
	}

	// ArchiveLabel can also unarchive directly.
	if err := s.ArchiveLabel(ctx, id, true); err != nil {
		t.Fatalf("ArchiveLabel 2: %v", err)
	}
	if err := s.ArchiveLabel(ctx, id, false); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	l, _ = s.GetLabel(ctx, id)
	if l.Archived {
		t.Fatalf("still archived: %+v", l)
	}
}

func TestMasterLabelUnion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// A workspace label may reuse a master name — both appear (union).
	if _, err := s.CreateLabel(ctx, 1, KindLabelIssue, LabelInput{Name: "Bug", Color: "#000000"}); err != nil {
		t.Fatalf("ws Bug: %v", err)
	}
	ls, err := s.ListLabels(ctx, 1, KindLabelIssue)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	var bugCount int
	for _, l := range ls {
		if l.Name == "Bug" {
			bugCount++
		}
	}
	if bugCount != 2 {
		t.Fatalf("Bug labels in ws1 = %d, want 2 (master + ws)", bugCount)
	}
}

func TestDiscussionLabels(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.SeedCategories(ctx); err != nil {
		t.Fatalf("SeedCategories: %v", err)
	}
	cats, err := s.ListDiscussionCategories(ctx)
	if err != nil {
		t.Fatalf("ListDiscussionCategories: %v", err)
	}

	did := mustDiscussionLabelKind(t, s)

	id, err := s.CreateDiscussion(ctx, 1, DiscussionInput{
		Title: "d1", Body: "b", CategoryID: cats[0].ID,
		LabelIDs: []int64{did},
	}, 0)
	if err != nil {
		t.Fatalf("CreateDiscussion: %v", err)
	}
	d, err := s.GetDiscussion(ctx, id)
	if err != nil {
		t.Fatalf("GetDiscussion: %v", err)
	}
	if len(d.Labels) != 1 || d.Labels[0].Name != "help-wanted" {
		t.Fatalf("labels = %+v", d.Labels)
	}

	// Update clears and re-attaches; an issue-kind label id is skipped.
	issueLabel := mustLabel(t, s, "issue-only", "#000000")
	if err := s.UpdateDiscussion(ctx, id, DiscussionInput{
		Title: "d1", Body: "b", CategoryID: cats[0].ID,
		LabelIDs: []int64{did, issueLabel},
	}); err != nil {
		t.Fatalf("UpdateDiscussion: %v", err)
	}
	d, _ = s.GetDiscussion(ctx, id)
	if len(d.Labels) != 1 || d.Labels[0].Name != "help-wanted" {
		t.Fatalf("after update labels = %+v", d.Labels)
	}

	// List populates labels too.
	ds, _, err := s.ListDiscussions(ctx, 1, DiscussionListFilter{})
	if err != nil {
		t.Fatalf("ListDiscussions: %v", err)
	}
	if len(ds) != 1 || len(ds[0].Labels) != 1 {
		t.Fatalf("list labels = %+v", ds)
	}
}

func mustDiscussionLabelKind(t *testing.T, s *Store) int64 {
	t.Helper()
	res, err := s.db.Exec(
		"INSERT INTO labels(workspace_id, kind, name, color) VALUES(1, 'discussion', 'help-wanted', '#ffaa00')")
	if err != nil {
		t.Fatalf("insert discussion label: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// TestMigrateV3FromV2 verifies a database stamped user_version=2 with the
// pre-managed labels table keeps its labels and issue_labels through the
// rebuild, and gains the four default master labels and discussion_labels.
func TestMigrateV3FromV2(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2.db")

	db, derr := sql.Open("sqlite", "file:"+path)
	if derr != nil {
		t.Fatalf("open raw db: %v", derr)
	}
	_, err := db.Exec(v2SchemaDDL + `
INSERT INTO labels(name, color) VALUES('Bug', '#d73a4a'), ('Custom', '#123456');
INSERT INTO issues(workspace_id, title, created_at, updated_at) VALUES(1, 'i1', 0, 0);
INSERT INTO issue_labels VALUES(1, 1);
PRAGMA user_version = 2;
`)
	if err != nil {
		t.Fatalf("build v2 db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v2 db: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open v2 db: %v", err)
	}
	defer s.Close()

	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil || v != schemaVersion {
		t.Fatalf("user_version = %d, want %d (err=%v)", v, schemaVersion, err)
	}

	ls, err := s.ListLabels(ctx, 1, KindLabelIssue)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	names := map[string]bool{}
	for _, l := range ls {
		names[l.Name] = true
		if l.WorkspaceID != 0 {
			t.Fatalf("label %q not a master row: %+v", l.Name, l)
		}
	}
	for _, want := range []string{"Bug", "Custom", "Documentation", "Enhancement", "Feature"} {
		if !names[want] {
			t.Fatalf("missing label %q: %v", want, names)
		}
	}

	// The preserved label id keeps its issue attachment.
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM issue_labels il JOIN labels l ON l.id = il.label_id
		WHERE il.issue_id = 1 AND l.name = 'Bug'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("issue_labels link lost: n=%d err=%v", n, err)
	}
}

// v2SchemaDDL is the pre-managed-labels schema (as user_version 2 stamped
// it) — the fixture migrateV3 must upgrade in place.
const v2SchemaDDL = `
CREATE TABLE IF NOT EXISTS workspaces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
INSERT INTO workspaces(slug, name, description, created_at)
	SELECT 'default', 'default', 'Default workspace.', 0
	WHERE NOT EXISTS (SELECT 1 FROM workspaces);
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS api_tokens (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS issues (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'closed')),
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	author_id INTEGER REFERENCES users(id)
);
CREATE TABLE IF NOT EXISTS comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	author_id INTEGER REFERENCES users(id),
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS labels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS issue_labels (
	issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
	PRIMARY KEY (issue_id, label_id)
);
CREATE TABLE IF NOT EXISTS discussion_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	slug TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	format TEXT NOT NULL DEFAULT 'open' CHECK(format IN ('open', 'qa'))
);
CREATE TABLE IF NOT EXISTS discussions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	category_id INTEGER NOT NULL REFERENCES discussion_categories(id),
	author_id INTEGER REFERENCES users(id),
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS discussion_threads (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	discussion_id INTEGER NOT NULL REFERENCES discussions(id) ON DELETE CASCADE,
	author_id INTEGER REFERENCES users(id),
	is_answer INTEGER NOT NULL DEFAULT 0,
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS discussion_comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	thread_id INTEGER NOT NULL REFERENCES discussion_threads(id) ON DELETE CASCADE,
	author_id INTEGER REFERENCES users(id),
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS changes (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	topic TEXT NOT NULL,
	kind TEXT NOT NULL,
	target INTEGER NOT NULL,
	parent INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
`

func TestArchivedLabelSurvivesIssueEdit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	lid := mustLabel(t, s, "legacy", "#000000")
	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "t", LabelIDs: []int64{lid}}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := s.ArchiveLabel(ctx, lid, true); err != nil {
		t.Fatalf("ArchiveLabel: %v", err)
	}

	// An unrelated edit cannot carry the archived label (pickers hide it),
	// so the replace must keep the attachment.
	if err := s.UpdateIssue(ctx, id, IssueInput{Title: "t2"}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	i, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if len(i.Labels) != 1 || i.Labels[0].Name != "legacy" || !i.Labels[0].Archived {
		t.Fatalf("labels = %+v", i.Labels)
	}

	// After a restore the label is selectable again and can be removed.
	if err := s.ArchiveLabel(ctx, lid, false); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if err := s.UpdateIssue(ctx, id, IssueInput{Title: "t3"}); err != nil {
		t.Fatalf("UpdateIssue 2: %v", err)
	}
	i, _ = s.GetIssue(ctx, id)
	if len(i.Labels) != 0 {
		t.Fatalf("labels after removal = %+v", i.Labels)
	}
}

func TestCrossWorkspaceLabelRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ws2, err := s.CreateWorkspace(ctx, WorkspaceInput{Slug: "other"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	other, err := s.CreateLabel(ctx, ws2, KindLabelIssue, LabelInput{Name: "foreign", Color: "#000000"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	// A master label stays attachable anywhere.
	master, err := s.CreateLabel(ctx, 0, KindLabelIssue, LabelInput{Name: "global-x", Color: "#111111"})
	if err != nil {
		t.Fatalf("CreateLabel master: %v", err)
	}

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "t", LabelIDs: []int64{other, master}}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	i, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if len(i.Labels) != 1 || i.Labels[0].Name != "global-x" || i.Labels[0].WorkspaceID != 0 || i.Labels[0].Kind != KindLabelIssue {
		t.Fatalf("labels = %+v, want only the master label with metadata", i.Labels)
	}

	// Update path enforces the same scope check via the issue's workspace.
	if err := s.UpdateIssue(ctx, id, IssueInput{Title: "t", LabelIDs: []int64{other}}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	i, _ = s.GetIssue(ctx, id)
	if len(i.Labels) != 0 {
		t.Fatalf("labels after update = %+v", i.Labels)
	}
}
