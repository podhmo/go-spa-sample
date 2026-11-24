// Labels are managed data rather than constants. A label belongs either
// to one workspace (workspace_id set) or to every workspace as shared
// master data (workspace_id NULL); kind separates the issue pool from the
// discussion pool. Listing a workspace's choices unions master and
// workspace rows — same-named labels from both scopes coexist rather than
// overriding each other. Deleting archives the row: it stays attached to
// existing issues/discussions but leaves pickers and filters.
package store

import (
	"context"
	"database/sql"
	"errors"

	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// isUniqueViolation reports the SQLite error produced by a UNIQUE
// constraint conflict.
func isUniqueViolation(err error) bool {
	var sqliteErr *sqliteDriver.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

const (
	KindLabelIssue      = "issue"
	KindLabelDiscussion = "discussion"
)

// ErrDuplicateLabel reports a create/update that would produce a second
// label with the same kind, name and scope.
var ErrDuplicateLabel = errors.New("store: label already exists")

// Label is a named, colored tag that can be attached to issues or
// discussions, depending on Kind. WorkspaceID is 0 for shared master
// labels. Archived labels keep their attachments but are hidden from
// pickers and filters.
type Label struct {
	ID          int64
	WorkspaceID int64 // 0 means master data shared by all workspaces
	Kind        string
	Name        string
	Color       string // CSS hex color, e.g. "#d73a4a"
	Archived    bool
}

// LabelInput carries validated form values for create/update. Kind and
// scope are fixed at creation and not part of the input.
type LabelInput struct {
	Name  string
	Color string
}

func scanLabel(s scanner) (*Label, error) {
	var l Label
	var ws sql.NullInt64
	var archived int
	if err := s.Scan(&l.ID, &ws, &l.Kind, &l.Name, &l.Color, &archived); err != nil {
		return nil, err
	}
	l.WorkspaceID = ws.Int64
	l.Archived = archived != 0
	return &l, nil
}

const labelColumns = "id, workspace_id, kind, name, color, archived"

// ListLabels returns the active labels of kind visible in workspace wsID:
// the shared master rows plus the workspace's own rows, ordered by name.
func (s *Store) ListLabels(ctx context.Context, wsID int64, kind string) ([]*Label, error) {
	return s.listLabels(ctx, "archived = 0", wsID, kind)
}

// ListArchivedLabels returns the archived counterpart of ListLabels, for
// the management page's archived section.
func (s *Store) ListArchivedLabels(ctx context.Context, wsID int64, kind string) ([]*Label, error) {
	return s.listLabels(ctx, "archived = 1", wsID, kind)
}

func (s *Store) listLabels(ctx context.Context, archivedCond string, wsID int64, kind string) ([]*Label, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+labelColumns+` FROM labels
		WHERE `+archivedCond+` AND kind = ? AND (workspace_id IS NULL OR workspace_id = ?)
		ORDER BY name`, kind, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Label
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetLabel returns one label by id (archived included), or ErrNotFound.
func (s *Store) GetLabel(ctx context.Context, id int64) (*Label, error) {
	l, err := scanLabel(s.db.QueryRowContext(ctx,
		"SELECT "+labelColumns+" FROM labels WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return l, err
}

// CreateLabel inserts a label of kind into workspace wsID (0 = master
// data shared by all workspaces) and returns its id. Creating a label
// whose name matches an archived row in the same scope restores that row
// with the new color; a live duplicate reports ErrDuplicateLabel.
func (s *Store) CreateLabel(ctx context.Context, wsID int64, kind string, in LabelInput) (int64, error) {
	var existingID int64
	var archived int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, archived FROM labels
		WHERE kind = ? AND name = ? AND COALESCE(workspace_id, 0) = ?`,
		kind, in.Name, wsID).Scan(&existingID, &archived)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := s.db.ExecContext(ctx,
			"INSERT INTO labels(workspace_id, kind, name, color) VALUES(?, ?, ?, ?)",
			nullID(wsID), kind, in.Name, in.Color)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	case err != nil:
		return 0, err
	case archived == 1:
		if _, err := s.db.ExecContext(ctx,
			"UPDATE labels SET archived = 0, color = ? WHERE id = ?", in.Color, existingID); err != nil {
			return 0, err
		}
		return existingID, nil
	default:
		return 0, ErrDuplicateLabel
	}
}

// UpdateLabel replaces a label's name and color. Kind and scope do not
// change. Returns ErrNotFound for a missing id and ErrDuplicateLabel when
// the new name collides in the same kind and scope.
func (s *Store) UpdateLabel(ctx context.Context, id int64, in LabelInput) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE labels SET name = ?, color = ? WHERE id = ?", in.Name, in.Color, id)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateLabel
		}
		return err
	}
	return checkAffected(res)
}

// ArchiveLabel sets a label's archived flag: archived labels leave all
// pickers and filters but keep their existing attachments; unarchiving
// restores them. Returns ErrNotFound for a missing id and
// ErrDuplicateLabel when unarchiving would collide with a live row.
func (s *Store) ArchiveLabel(ctx context.Context, id int64, archived bool) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE labels SET archived = ? WHERE id = ?", archived, id)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateLabel
		}
		return err
	}
	return checkAffected(res)
}
