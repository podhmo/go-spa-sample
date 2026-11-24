package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

const (
	StatusOpen   = "open"
	StatusClosed = "closed"
)

// Issue is a single tracker entry belonging to one workspace. Labels is
// populated by read methods. Author is the posting account, or nil for an
// anonymous post.
type Issue struct {
	ID          int64
	WorkspaceID int64
	Title       string
	Body        string
	Status      string // StatusOpen or StatusClosed
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Labels      []Label
	Author      *User
}

// Comment is a plaintext comment on an issue, shown in chronological order.
type Comment struct {
	ID        int64
	IssueID   int64
	Body      string
	CreatedAt time.Time
	Author    *User
	Moderation
}

// IssueInput carries validated form values for create/update.
type IssueInput struct {
	Title    string
	Body     string
	LabelIDs []int64
}

// ListFilter controls ListIssues. Zero values apply the defaults documented
// per field.
type ListFilter struct {
	Status  string // "" means StatusOpen; "all" disables status filtering
	Query   string // case-insensitive substring match on title OR body
	LabelID int64  // 0 means no label filter
	Sort    string // "newest" (default, created_at DESC) or "oldest" (ASC)
	Page    int    // 1-based; values < 1 are clamped to 1
	PerPage int    // <= 0 defaults to 20
}

// ListIssues returns the page of issues in workspace wsID matching f
// (each with Labels populated) and the total number of matching issues
// before pagination.
func (s *Store) ListIssues(ctx context.Context, wsID int64, f ListFilter) ([]*Issue, int, error) {
	var conds []string
	var args []any

	conds = append(conds, "i.workspace_id = ?")
	args = append(args, wsID)

	status := f.Status
	if status == "" {
		status = StatusOpen
	}
	if status != "all" {
		conds = append(conds, "i.status = ?")
		args = append(args, status)
	}
	if f.Query != "" {
		conds = append(conds, "(instr(lower(i.title), lower(?)) > 0 OR instr(lower(i.body), lower(?)) > 0)")
		args = append(args, f.Query, f.Query)
	}
	if f.LabelID > 0 {
		conds = append(conds, "EXISTS (SELECT 1 FROM issue_labels il WHERE il.issue_id = i.id AND il.label_id = ?)")
		args = append(args, f.LabelID)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM issues i"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dir := "DESC"
	if f.Sort == "oldest" {
		dir = "ASC"
	}
	page := max(f.Page, 1)
	perPage := f.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	q := `SELECT i.id, i.workspace_id, i.title, i.body, i.status, i.created_at, i.updated_at,
		u.id, u.name, u.created_at
		FROM issues i LEFT JOIN users u ON u.id = i.author_id` + where +
		" ORDER BY i.created_at " + dir + ", i.id " + dir + " LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, err
	}
	var issues []*Issue
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		issues = append(issues, i)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()

	if err := s.attachLabels(ctx, issues); err != nil {
		return nil, 0, err
	}
	return issues, total, nil
}

// GetIssue returns one issue with Labels, or ErrNotFound. The lookup is
// global — callers that need workspace scoping compare Issue.WorkspaceID
// against their context.
func (s *Store) GetIssue(ctx context.Context, id int64) (*Issue, error) {
	i, err := scanIssue(s.db.QueryRowContext(ctx,
		`SELECT i.id, i.workspace_id, i.title, i.body, i.status, i.created_at, i.updated_at,
			u.id, u.name, u.created_at
			FROM issues i LEFT JOIN users u ON u.id = i.author_id WHERE i.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.attachLabels(ctx, []*Issue{i}); err != nil {
		return nil, err
	}
	return i, nil
}

// CreateIssue inserts an open issue in workspace wsID and returns its
// new ID (also the issue number; IDs are never reused and stay global
// across workspaces). authorID is the posting user's ID, or 0 for an
// anonymous post.
func (s *Store) CreateIssue(ctx context.Context, wsID int64, in IssueInput, authorID int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO issues(workspace_id, title, body, status, created_at, updated_at, author_id) VALUES(?, ?, ?, ?, ?, ?, ?)",
		wsID, in.Title, in.Body, StatusOpen, now, now, nullID(authorID))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := setIssueLabels(ctx, tx, id, wsID, in.LabelIDs); err != nil {
		return 0, err
	}
	if err := s.commitTx(ctx, tx, events.Change{Topic: events.IssuesTopic, Kind: events.KindNew, Target: id}); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateIssue replaces title/body/labels and bumps UpdatedAt. Returns
// ErrNotFound if the issue does not exist.
func (s *Store) UpdateIssue(ctx context.Context, id int64, in IssueInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		"UPDATE issues SET title = ?, body = ?, updated_at = ? WHERE id = ?",
		in.Title, in.Body, time.Now().UTC().Unix(), id)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	var wsID int64
	if err := tx.QueryRowContext(ctx, "SELECT workspace_id FROM issues WHERE id = ?", id).Scan(&wsID); err != nil {
		return err
	}
	if err := setIssueLabels(ctx, tx, id, wsID, in.LabelIDs); err != nil {
		return err
	}
	if err := s.commitTx(ctx, tx, events.Change{Topic: events.IssueTopic(id), Kind: events.KindEdit, Target: id}); err != nil {
		return err
	}
	return nil
}

// SetIssueStatus sets status to StatusOpen or StatusClosed. Returns
// ErrNotFound if the issue does not exist.
func (s *Store) SetIssueStatus(ctx context.Context, id int64, status string) error {
	if status != StatusOpen && status != StatusClosed {
		return fmt.Errorf("store: invalid status %q", status)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// updated_at is bumped only when the status actually changes; a no-op
	// post (e.g. closing an already-closed issue) leaves it alone. The
	// updated_at clause precedes the status assignment so the CASE compares
	// against the pre-update status. An existing row always matches the
	// UPDATE, so RowsAffected still reports 0 only when the issue is missing.
	res, err := tx.ExecContext(ctx,
		"UPDATE issues SET updated_at = CASE WHEN status != ? THEN ? ELSE updated_at END, status = ? WHERE id = ?",
		status, time.Now().UTC().Unix(), status, id)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx, events.Change{Topic: events.IssueTopic(id), Kind: events.KindStatus, Target: id})
}

// DeleteIssue removes the issue along with its comments and label links.
// Returns ErrNotFound if the issue does not exist.
func (s *Store) DeleteIssue(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, "DELETE FROM issues WHERE id = ?", id)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.IssueTopic(id), Kind: events.KindDelete, Target: id},
		events.Change{Topic: events.IssuesTopic, Kind: events.KindDelete, Target: id})
}

// ListComments returns comments for an issue in chronological order.
func (s *Store) ListComments(ctx context.Context, issueID int64) ([]*Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.issue_id, c.body, c.created_at,
			c.edited_at, c.hidden_at, c.hidden_reason,
			u.id, u.name, u.created_at
			FROM comments c LEFT JOIN users u ON u.id = c.author_id
			WHERE c.issue_id = ? ORDER BY c.created_at ASC, c.id ASC`,
		issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateComment appends a comment. Returns ErrNotFound if the issue is
// missing. authorID is the posting user's ID, or 0 for an anonymous post.
func (s *Store) CreateComment(ctx context.Context, issueID int64, body string, authorID int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM issues WHERE id = ?", issueID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO comments(issue_id, body, created_at, author_id) VALUES(?, ?, ?, ?)",
		issueID, body, now, nullID(authorID))
	if err != nil {
		return 0, err
	}
	// A new comment is activity on the issue, so bump its updated_at with
	// the comment's timestamp.
	if _, err := tx.ExecContext(ctx,
		"UPDATE issues SET updated_at = ? WHERE id = ?", now, issueID); err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := s.commitTx(ctx, tx, events.Change{Topic: events.IssueTopic(issueID), Kind: events.KindComment, Target: id}); err != nil {
		return 0, err
	}
	return id, nil
}

// attachLabels populates Labels on each issue with a single grouped query.
func (s *Store) attachLabels(ctx context.Context, issues []*Issue) error {
	if len(issues) == 0 {
		return nil
	}
	byID := make(map[int64]*Issue, len(issues))
	ph := make([]string, len(issues))
	args := make([]any, len(issues))
	for i, is := range issues {
		byID[is.ID] = is
		ph[i] = "?"
		args[i] = is.ID
	}
	q := `SELECT il.issue_id, l.id, l.name, l.color, l.kind, COALESCE(l.workspace_id, 0), l.archived
		FROM issue_labels il JOIN labels l ON l.id = il.label_id
		WHERE il.issue_id IN (` + strings.Join(ph, ",") + `) ORDER BY l.name`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var issueID int64
		var l Label
		if err := rows.Scan(&issueID, &l.ID, &l.Name, &l.Color, &l.Kind, &l.WorkspaceID, &l.Archived); err != nil {
			return err
		}
		if is := byID[issueID]; is != nil {
			is.Labels = append(is.Labels, l)
		}
	}
	return rows.Err()
}

// setIssueLabels replaces the label set of an issue inside tx.
// Attachments to archived labels survive the replace: they are not
// selectable in pickers, so an unrelated edit cannot carry them and must
// not drop them. Only master (workspace_id NULL) and the issue's own
// workspace labels may be attached.
func setIssueLabels(ctx context.Context, tx *sql.Tx, issueID, wsID int64, labelIDs []int64) error {
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM issue_labels WHERE issue_id = ? AND label_id NOT IN (SELECT id FROM labels WHERE archived = 1)",
		issueID); err != nil {
		return err
	}
	for _, lid := range labelIDs {
		// Label ids that do not exist in labels make the SELECT yield no
		// row (nothing inserted); duplicates hit the PRIMARY KEY conflict
		// that OR IGNORE skips. Both are silently dropped rather than
		// failing with a constraint error.
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO issue_labels(issue_id, label_id)
				SELECT ?, id FROM labels WHERE id = ? AND kind = '`+KindLabelIssue+`' AND archived = 0
				AND (workspace_id IS NULL OR workspace_id = ?)`,
			issueID, lid, wsID); err != nil {
			return err
		}
	}
	return nil
}

func scanIssue(s scanner) (*Issue, error) {
	var i Issue
	var c, u int64
	var aid sql.NullInt64
	var aname sql.NullString
	var acreated sql.NullInt64
	if err := s.Scan(&i.ID, &i.WorkspaceID, &i.Title, &i.Body, &i.Status, &c, &u, &aid, &aname, &acreated); err != nil {
		return nil, err
	}
	i.CreatedAt = time.Unix(c, 0).UTC()
	i.UpdatedAt = time.Unix(u, 0).UTC()
	i.Author = scanUser(aid, aname, acreated)
	return &i, nil
}

func scanComment(s scanner) (*Comment, error) {
	var c Comment
	var ts int64
	var ed, hid sql.NullInt64
	var reason sql.NullString
	var aid sql.NullInt64
	var aname sql.NullString
	var acreated sql.NullInt64
	if err := s.Scan(&c.ID, &c.IssueID, &c.Body, &ts, &ed, &hid, &reason, &aid, &aname, &acreated); err != nil {
		return nil, err
	}
	c.CreatedAt = time.Unix(ts, 0).UTC()
	c.Moderation = scanModeration(ed, hid, reason)
	c.Author = scanUser(aid, aname, acreated)
	return &c, nil
}
