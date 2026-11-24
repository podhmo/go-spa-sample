// Workspaces: GitHub-repository-like containers (see schemaDDL in
// store.go). Every issue and discussion belongs to exactly one workspace; deleting a workspace cascades to its contents like a
// GitHub repository deletion. Design: docs/sketch/ja/workspaces.md.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrDuplicateSlug reports a workspace slug that is already taken (slugs
// are unique — they are the URL path segment of the workspace).
var ErrDuplicateSlug = errors.New("store: workspace slug already taken")

// ErrLastWorkspace reports a delete attempt on the only remaining
// workspace — a tracker with no container cannot accept content.
var ErrLastWorkspace = errors.New("store: cannot delete the last workspace")

// DefaultWorkspaceSlug is the slug of the workspace the schema creates
// and EnsureDefaultWorkspace re-creates when no workspace exists.
const DefaultWorkspaceSlug = "default"

// Workspace is a container of issues and discussions. IssueCount and
// DiscussionCount are populated only by ListWorkspaces and the BySlug/ByID
// lookups — 0 on a freshly created workspace.
type Workspace struct {
	ID              int64
	Slug            string
	Name            string
	Description     string
	CreatedAt       time.Time
	IssueCount      int
	DiscussionCount int
}

// WorkspaceInput carries validated form values for creation. An empty Name
// falls back to the slug (it is the display name, not the identity).
type WorkspaceInput struct {
	Slug        string
	Name        string
	Description string
}

// ListWorkspaces returns every workspace ordered by slug, each with its
// issue and discussion counts populated.
func (s *Store) ListWorkspaces(ctx context.Context) ([]*Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.slug, w.name, w.description, w.created_at,
			(SELECT COUNT(*) FROM issues i WHERE i.workspace_id = w.id),
			(SELECT COUNT(*) FROM discussions d WHERE d.workspace_id = w.id)
		FROM workspaces w ORDER BY w.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Workspace
	for rows.Next() {
		ws, err := scanWorkspace(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}

// WorkspaceBySlug returns one workspace by slug, or ErrNotFound.
func (s *Store) WorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error) {
	return s.findWorkspace(ctx, "slug", slug)
}

// WorkspaceByID returns one workspace by id, or ErrNotFound.
func (s *Store) WorkspaceByID(ctx context.Context, id int64) (*Workspace, error) {
	return s.findWorkspace(ctx, "id", id)
}

func (s *Store) findWorkspace(ctx context.Context, col string, key any) (*Workspace, error) {
	ws, err := scanWorkspace(s.db.QueryRowContext(ctx,
		`SELECT w.id, w.slug, w.name, w.description, w.created_at,
			(SELECT COUNT(*) FROM issues i WHERE i.workspace_id = w.id),
			(SELECT COUNT(*) FROM discussions d WHERE d.workspace_id = w.id)
		FROM workspaces w WHERE w.`+col+` = ?`, key), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return ws, nil
}

// CreateWorkspace inserts a workspace and returns its id. Returns
// ErrDuplicateSlug when the slug is taken.
func (s *Store) CreateWorkspace(ctx context.Context, in WorkspaceInput) (int64, error) {
	name := in.Name
	if name == "" {
		name = in.Slug
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO workspaces(slug, name, description, created_at) VALUES(?, ?, ?, ?)",
		in.Slug, name, in.Description, time.Now().UTC().Unix())
	if err != nil {
		var sqliteErr *sqliteDriver.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return 0, ErrDuplicateSlug
		}
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteWorkspace removes the workspace; its issues and discussions — and
// through them their comments, threads, replies and label links — go via
// ON DELETE CASCADE. Returns ErrNotFound for an unknown slug and
// ErrLastWorkspace when it is the only workspace left.
func (s *Store) DeleteWorkspace(ctx context.Context, slug string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM workspaces WHERE slug = ?", slug).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces").Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastWorkspace
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM workspaces WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// EnsureDefaultWorkspace creates the 'default' workspace, but only when
// no workspaces exist at all — it never resurrects a deliberately deleted
// default while other workspaces remain. init and server startup both
// call this so an emptied tracker regains a container.
func (s *Store) EnsureDefaultWorkspace(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO workspaces(slug, name, description, created_at)
	SELECT 'default', 'default', 'Default workspace.', ?
	WHERE NOT EXISTS (SELECT 1 FROM workspaces)`, time.Now().UTC().Unix())
	return err
}

func scanWorkspace(s scanner, withCounts bool) (*Workspace, error) {
	var w Workspace
	var ts int64
	if err := s.Scan(&w.ID, &w.Slug, &w.Name, &w.Description, &ts,
		&w.IssueCount, &w.DiscussionCount); err != nil {
		return nil, err
	}
	w.CreatedAt = time.Unix(ts, 0).UTC()
	return &w, nil
}
