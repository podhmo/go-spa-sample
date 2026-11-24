// Pull requests: a DB row is the PR's identity — (base_ref, head_ref)
// plus conversation data — while commits, diffs and conflicts are always
// computed from git on the fly (docs/sketch/ja/pull-requests.md). Closing
// records the final SHAs so the diff stays reproducible; the git side
// pins refs/zatudan/pr/<id>/head to keep the commits reachable, and that
// pin is applied through the create/close/merge calls below.
package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

// Pull request statuses — the status column's CHECK mirrors them.
const (
	PRStatusOpen   = "open"
	PRStatusClosed = "closed"
	PRStatusMerged = "merged"
)

var (
	// ErrDuplicatePR reports a create that would open a second PR for the
	// same (base, head) pair — raised by the partial unique index.
	ErrDuplicatePR = errors.New("store: an open pull request already exists for this base/head")
	// ErrInvalidPRTransition reports a status change the PR lifecycle
	// forbids: merged is terminal (GitHub-like), closed only reopens,
	// and only an open PR can close or merge.
	ErrInvalidPRTransition = errors.New("store: invalid pull request status transition")
	// ErrSameRef reports a create whose base and head normalized to the
	// same ref — a PR with an empty range is meaningless.
	ErrSameRef = errors.New("store: base and head are the same ref")
	// ErrInvalidAnchor reports a review comment whose anchor fields are
	// inconsistent — path and line must be set together or both empty.
	ErrInvalidAnchor = errors.New("store: inconsistent comment anchor")
)

// PullRequest is one PR row. The sha fields are "" until recorded:
// BaseSHA is resolved at create (display aid), HeadSHA/MergeBaseSHA at
// close or merge, MergeSHA/MergeMethod at merge only.
type PullRequest struct {
	ID           int64
	BaseRef      string // normalized ref name or sha
	HeadRef      string
	Title        string
	Body         string
	Status       string // PRStatus*
	Author       *User  // nil for an anonymous post
	BaseSHA      string
	HeadSHA      string
	MergeBaseSHA string
	MergeSHA     string
	MergeMethod  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ClosedAt     *time.Time
}

// Open reports whether the PR accepts comments and a merge.
func (p *PullRequest) Open() bool { return p.Status == PRStatusOpen }

// PRComment is one conversation entry on a PR. A zero anchor (Path "")
// is a plain comment; a set anchor (Path+Line, plus Side/AnchorSHA/
// DiffSnippet) is a review comment on a specific diff line.
type PRComment struct {
	ID          int64
	PRID        int64
	Path        string // "" = conversation comment
	Line        int    // 1-based line of AnchorSHA's blob; 0 with Path ""
	Side        string // "right" (new side) or "left" (old/deleted side)
	AnchorSHA   string
	DiffSnippet string
	Body        string
	CreatedAt   time.Time
	Author      *User
	Moderation
}

// Anchored reports whether the comment is a review comment.
func (c *PRComment) Anchored() bool { return c.Path != "" }

// PRInput carries validated values for create/update. BaseRef/HeadRef are
// already normalized (symbolic-full-name or sha) by the caller — the
// store never runs git. BaseSHA/HeadSHA are the create-time resolutions
// recorded for display.
type PRInput struct {
	BaseRef string
	HeadRef string
	BaseSHA string
	HeadSHA string
	Title   string
	Body    string
}

// PRCommentInput carries a new comment; anchor fields are "" / 0 for a
// plain conversation comment.
type PRCommentInput struct {
	Path        string
	Line        int
	Side        string // "" defaults to "right"
	AnchorSHA   string
	DiffSnippet string
	Body        string
}

// PRListFilter controls ListPullRequests. Zero values apply the defaults
// documented per field.
type PRListFilter struct {
	Status  string // "open" (default), "closed", "merged", or "all"
	Query   string // case-insensitive substring match on title OR body
	Sort    string // "newest" (default) or "oldest"
	Page    int    // 1-based; values < 1 are clamped to 1
	PerPage int    // <= 0 defaults to 20
}

// prColumns lists pull_requests in SELECT order shared by the scans.
const prColumns = `p.id, p.base_ref, p.head_ref, p.title, p.body, p.status,
	p.base_sha, p.head_sha, p.merge_base_sha, p.merge_sha, p.merge_method,
	p.created_at, p.updated_at, p.closed_at,
	u.id, u.name, u.created_at`

// ListPullRequests returns the page matching f plus the total before
// pagination. PRs are a top-level resource — no workspace filter.
func (s *Store) ListPullRequests(ctx context.Context, f PRListFilter) ([]*PullRequest, int, error) {
	var conds []string
	var args []any
	switch f.Status {
	case "", PRStatusOpen:
		conds = append(conds, "p.status = 'open'")
	case "all":
	case PRStatusClosed, PRStatusMerged:
		conds = append(conds, "p.status = ?")
		args = append(args, f.Status)
	default:
		// Unknown filters behave like "all" rather than erroring, the
		// same tolerance the issue list's ?status= shows.
	}
	if f.Query != "" {
		conds = append(conds, "(instr(lower(p.title), lower(?)) > 0 OR instr(lower(p.body), lower(?)) > 0)")
		args = append(args, f.Query, f.Query)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pull_requests p"+where, args...).Scan(&total); err != nil {
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
	q := `SELECT ` + prColumns + `
		FROM pull_requests p LEFT JOIN users u ON u.id = p.author_id` + where +
		" ORDER BY p.created_at " + dir + ", p.id " + dir + " LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*PullRequest
	for rows.Next() {
		p, err := scanPullRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// GetPullRequest returns one PR or ErrNotFound.
func (s *Store) GetPullRequest(ctx context.Context, id int64) (*PullRequest, error) {
	p, err := scanPullRequest(s.db.QueryRowContext(ctx,
		`SELECT `+prColumns+`
		FROM pull_requests p LEFT JOIN users u ON u.id = p.author_id
		WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// CreatePullRequest inserts an open PR and returns its id. pin is invoked
// inside the same transaction with the new id — the caller pins
// refs/zatudan/pr/<id>/head there, so a failed pin rolls the insert back
// instead of leaving an unprotected PR. base == head is rejected before
// insert; a second open PR for the same pair reports ErrDuplicatePR via
// the partial unique index.
func (s *Store) CreatePullRequest(ctx context.Context, in PRInput, authorID int64, pin func(prID int64) error) (int64, error) {
	if in.BaseRef == in.HeadRef {
		return 0, ErrSameRef
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO pull_requests(base_ref, head_ref, title, body, base_sha, head_sha, author_id, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.BaseRef, in.HeadRef, in.Title, in.Body, in.BaseSHA, in.HeadSHA, nullID(authorID), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrDuplicatePR
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if pin != nil {
		if err := pin(id); err != nil {
			return 0, err
		}
	}
	if err := s.commitTx(ctx, tx,
		events.Change{Topic: events.PullsTopic, Kind: events.KindNew, Target: id}); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdatePullRequest replaces title/body and bumps UpdatedAt. Refs are
// immutable after creation (the pin and the live resolution both assume
// it), matching the sketch's "edit = メタデータ" scope.
func (s *Store) UpdatePullRequest(ctx context.Context, id int64, in PRInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		"UPDATE pull_requests SET title = ?, body = ?, updated_at = ? WHERE id = ?",
		in.Title, in.Body, time.Now().UTC().Unix(), id)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(id), Kind: events.KindEdit, Target: id})
}

// DeletePullRequest removes the PR and its comments (CASCADE).
func (s *Store) DeletePullRequest(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "DELETE FROM pull_requests WHERE id = ?", id)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(id), Kind: events.KindDelete, Target: id},
		events.Change{Topic: events.PullsTopic, Kind: events.KindDelete, Target: id})
}

// ClosePullRequest records the final head/merge-base resolution and moves
// open → closed. headSHA/mergeBaseSHA are resolved by the caller first so
// the stored values and the pin update describe the same moment.
// ErrInvalidPRTransition when the PR is not open.
func (s *Store) ClosePullRequest(ctx context.Context, id int64, headSHA, mergeBaseSHA string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE pull_requests SET status = 'closed', head_sha = ?, merge_base_sha = ?,
			updated_at = ?, closed_at = ? WHERE id = ? AND status = 'open'`,
		headSHA, mergeBaseSHA, time.Now().UTC().Unix(), time.Now().UTC().Unix(), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Existence must be probed through the tx: :memory: pins a
		// single connection, so s.db queries would deadlock here.
		var one int
		err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM pull_requests WHERE id = ?", id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return ErrInvalidPRTransition
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(id), Kind: events.KindStatus, Target: id},
		events.Change{Topic: events.PullsTopic, Kind: events.KindStatus, Target: id})
}

// ReopenPullRequest moves closed → open, clearing closed_at. Merged is
// terminal — ErrInvalidPRTransition there and from open.
func (s *Store) ReopenPullRequest(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE pull_requests SET status = 'open', updated_at = ?, closed_at = NULL
			WHERE id = ? AND status = 'closed'`,
		time.Now().UTC().Unix(), id)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicatePR
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Existence must be probed through the tx: :memory: pins a
		// single connection, so s.db queries would deadlock here.
		var one int
		err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM pull_requests WHERE id = ?", id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return ErrInvalidPRTransition
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(id), Kind: events.KindStatus, Target: id},
		events.Change{Topic: events.PullsTopic, Kind: events.KindStatus, Target: id})
}

// PRMergeInfo records the git-side merge result the caller already
// performed (gitrepo.Merge ran before this write).
type PRMergeInfo struct {
	MergeSHA     string
	MergeMethod  string
	HeadSHA      string
	MergeBaseSHA string
}

// MarkMerged moves open → merged with the merge's resulting SHAs.
// ErrInvalidPRTransition when the PR is not open.
func (s *Store) MarkMerged(ctx context.Context, id int64, info PRMergeInfo) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		`UPDATE pull_requests SET status = 'merged', merge_sha = ?, merge_method = ?,
			head_sha = ?, merge_base_sha = ?, updated_at = ?, closed_at = ?
			WHERE id = ? AND status = 'open'`,
		info.MergeSHA, info.MergeMethod, info.HeadSHA, info.MergeBaseSHA, now, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Existence must be probed through the tx: :memory: pins a
		// single connection, so s.db queries would deadlock here.
		var one int
		err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM pull_requests WHERE id = ?", id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return ErrInvalidPRTransition
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(id), Kind: events.KindStatus, Target: id},
		events.Change{Topic: events.PullsTopic, Kind: events.KindStatus, Target: id})
}

// prCommentColumns lists pr_comments in the order scanPRComment expects
// (moderation triple sits before the author columns like everywhere).
const prCommentColumns = `c.id, c.pr_id, c.path, c.line, c.side, c.anchor_sha, c.diff_snippet, c.body,
	c.edited_at, c.hidden_at, c.hidden_reason, c.created_at,
	u.id, u.name, u.created_at`

// ListPRComments returns the PR's conversation in chronological order —
// plain comments and anchored review comments share one timeline.
func (s *Store) ListPRComments(ctx context.Context, prID int64) ([]*PRComment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+prCommentColumns+`
		FROM pr_comments c LEFT JOIN users u ON u.id = c.author_id
		WHERE c.pr_id = ? ORDER BY c.created_at ASC, c.id ASC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PRComment
	for rows.Next() {
		c, err := scanPRComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreatePRComment appends a comment and bumps the PR's updated_at (a new
// post is activity — the issue/discussion rule). Anchor consistency
// (path ⇔ line set) is enforced by the schema CHECK; this validates it
// first for a friendlier error. ErrNotFound when the PR is missing.
func (s *Store) CreatePRComment(ctx context.Context, prID int64, in PRCommentInput, authorID int64) (int64, error) {
	if (in.Path == "") != (in.Line == 0) {
		return 0, ErrInvalidAnchor
	}
	side := in.Side
	if side == "" {
		side = "right"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM pull_requests WHERE id = ?", prID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO pr_comments(pr_id, path, line, side, anchor_sha, diff_snippet, body, author_id, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		prID, nullStr(in.Path), nullInt(int64(in.Line)), side, nullStr(in.AnchorSHA), nullStr(in.DiffSnippet), in.Body, nullID(authorID), now)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE pull_requests SET updated_at = ? WHERE id = ?", now, prID); err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(prID), Kind: events.KindComment, Target: id}); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdatePRComment replaces a comment's body, stamping edited_at on a real
// change. ErrNotFound when missing or belonging to a different PR.
func (s *Store) UpdatePRComment(ctx context.Context, prID, commentID int64, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE pr_comments SET body = ?,
			edited_at = CASE WHEN body != ? THEN ? ELSE edited_at END
			WHERE id = ? AND pr_id = ?`,
		body, body, time.Now().UTC().Unix(), commentID, prID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(prID), Kind: events.KindComment, Target: commentID})
}

// DeletePRComment removes a comment permanently.
func (s *Store) DeletePRComment(ctx context.Context, prID, commentID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		"DELETE FROM pr_comments WHERE id = ? AND pr_id = ?", commentID, prID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(prID), Kind: events.KindRemove, Target: commentID})
}

// SetPRCommentHidden minimizes a comment with a required reason, or
// restores it when reason is "". ErrInvalidHideReason/ErrNotFound.
func (s *Store) SetPRCommentHidden(ctx context.Context, prID, commentID int64, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := setHidden(ctx, tx, "pr_comments", "pr_id", prID, commentID, reason)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.PullRequestTopic(prID), Kind: events.KindComment, Target: commentID})
}

// nullStr maps "" to SQL NULL for the anchor fields whose CHECK needs
// real NULLs.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullInt maps 0 to SQL NULL (the anchor's line column).
func nullInt(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func scanPullRequest(s scanner) (*PullRequest, error) {
	var p PullRequest
	var baseSHA, headSHA, mbaseSHA, mergeSHA, method sql.NullString
	var created, updated int64
	var closed sql.NullInt64
	var aid sql.NullInt64
	var aname sql.NullString
	var acreated sql.NullInt64
	if err := s.Scan(&p.ID, &p.BaseRef, &p.HeadRef, &p.Title, &p.Body, &p.Status,
		&baseSHA, &headSHA, &mbaseSHA, &mergeSHA, &method,
		&created, &updated, &closed,
		&aid, &aname, &acreated); err != nil {
		return nil, err
	}
	p.BaseSHA, p.HeadSHA = baseSHA.String, headSHA.String
	p.MergeBaseSHA, p.MergeSHA, p.MergeMethod = mbaseSHA.String, mergeSHA.String, method.String
	p.CreatedAt, p.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	if closed.Valid {
		t := time.Unix(closed.Int64, 0).UTC()
		p.ClosedAt = &t
	}
	p.Author = scanUser(aid, aname, acreated)
	return &p, nil
}

func scanPRComment(s scanner) (*PRComment, error) {
	var c PRComment
	var path sql.NullString
	var line sql.NullInt64
	var anchorSHA, snippet sql.NullString
	var ed, hid sql.NullInt64
	var reason sql.NullString
	var ts int64
	var aid sql.NullInt64
	var aname sql.NullString
	var acreated sql.NullInt64
	if err := s.Scan(&c.ID, &c.PRID, &path, &line, &c.Side, &anchorSHA, &snippet, &c.Body,
		&ed, &hid, &reason, &ts,
		&aid, &aname, &acreated); err != nil {
		return nil, err
	}
	c.Path = path.String
	c.Line = int(line.Int64)
	c.AnchorSHA = anchorSHA.String
	c.DiffSnippet = snippet.String
	c.CreatedAt = time.Unix(ts, 0).UTC()
	c.Moderation = scanModeration(ed, hid, reason)
	c.Author = scanUser(aid, aname, acreated)
	return &c, nil
}
