// Discussions: GitHub Discussions-style open conversations (see migrations
// in store.go). Unlike issues, discussions never close —
// a Q&A-category discussion reports "answered" through a marked thread
// instead. A discussion holds threads; a thread is its opening post plus
// a flat list of comment replies. Design: docs/sketch/ja/discussions.md
// (案B: 並列テーブル), restructured per docs/ja/decisions.md.
package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

// ErrInvalidCategory reports a discussion input referencing a category
// that does not exist (e.g. a hand-crafted POST).
var ErrInvalidCategory = errors.New("store: unknown discussion category")

// ErrInvalidThread reports a comment reply whose target thread is missing
// or belongs to another discussion.
var ErrInvalidThread = errors.New("store: reply target is not a thread of this discussion")

// ErrNotAnswerable reports an answer mark on a thread of a discussion
// whose category is not qa-format.
var ErrNotAnswerable = errors.New("store: thread cannot be marked as answer")

const (
	CategoryFormatOpen = "open"
	CategoryFormatQA   = "qa"
)

// DiscussionCategory is a seed-defined classification; every discussion
// belongs to exactly one. Format "qa" makes its discussions answerable.
type DiscussionCategory struct {
	ID          int64
	Name        string
	Slug        string // used in ?category= URLs
	Description string
	Format      string // CategoryFormatOpen or CategoryFormatQA
}

// Discussion is one conversation. Answered reports whether any of
// its threads is marked as the answer (only meaningful for qa categories).
type Discussion struct {
	ID          int64
	WorkspaceID int64
	Title       string
	Body        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Category    DiscussionCategory
	Author      *User // nil for an anonymous post
	Answered    bool
	Labels      []Label // populated by ListDiscussions and GetDiscussion
}

// DiscussionThread is one conversation thread inside a discussion. The
// thread itself is the opening post (body/author/is_answer live here);
// Comments holds its replies, populated by ListDiscussionThreads. Only
// threads — never replies — can be marked as the answer, so the schema
// keeps one level of nesting by construction.
type DiscussionThread struct {
	ID           int64
	DiscussionID int64
	IsAnswer     bool
	Body         string
	CreatedAt    time.Time
	Author       *User // nil for an anonymous post
	Comments     []*DiscussionComment
	Moderation
}

// DiscussionComment is a reply inside a thread.
type DiscussionComment struct {
	ID        int64
	ThreadID  int64
	Body      string
	CreatedAt time.Time
	Author    *User // nil for an anonymous post
	Moderation
}

// DiscussionInput carries validated form values for create/update.
type DiscussionInput struct {
	Title      string
	Body       string
	CategoryID int64
	LabelIDs   []int64
}

// DiscussionListFilter controls ListDiscussions. Zero values apply the
// defaults documented per field.
type DiscussionListFilter struct {
	CategorySlug string // "" means all categories
	Answered     string // "", "yes" (has an answer thread), or "no"
	Query        string // case-insensitive substring match on title OR body
	Sort         string // "newest" (default, created_at DESC) or "oldest" (ASC)
	Page         int    // 1-based; values < 1 are clamped to 1
	PerPage      int    // <= 0 defaults to 20
}

// ListDiscussionCategories returns all categories ordered by name.
func (s *Store) ListDiscussionCategories(ctx context.Context) ([]*DiscussionCategory, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, slug, description, format FROM discussion_categories ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*DiscussionCategory
	for rows.Next() {
		var c DiscussionCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Format); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// ListDiscussions returns the page of discussions in workspace wsID
// matching f (each with Category and Answered populated) and the total
// before pagination.
func (s *Store) ListDiscussions(ctx context.Context, wsID int64, f DiscussionListFilter) ([]*Discussion, int, error) {
	var conds []string
	var args []any

	conds = append(conds, "d.workspace_id = ?")
	args = append(args, wsID)

	if f.CategorySlug != "" {
		conds = append(conds, "c.slug = ?")
		args = append(args, f.CategorySlug)
	}
	if f.Query != "" {
		conds = append(conds, "(instr(lower(d.title), lower(?)) > 0 OR instr(lower(d.body), lower(?)) > 0)")
		args = append(args, f.Query, f.Query)
	}
	switch f.Answered {
	case "yes":
		conds = append(conds, discussionAnsweredCond)
	case "no":
		conds = append(conds, "NOT "+discussionAnsweredCond)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM discussions d JOIN discussion_categories c ON c.id = d.category_id"+where,
		args...).Scan(&total); err != nil {
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

	q := `SELECT d.id, d.workspace_id, d.title, d.body, d.created_at, d.updated_at,
		c.id, c.name, c.slug, c.description, c.format,
		u.id, u.name, u.created_at,
		` + discussionAnsweredCond + `
		FROM discussions d
		JOIN discussion_categories c ON c.id = d.category_id
		LEFT JOIN users u ON u.id = d.author_id` + where +
		" ORDER BY d.created_at " + dir + ", d.id " + dir + " LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*Discussion
	for rows.Next() {
		d, err := scanDiscussion(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := s.attachDiscussionLabels(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// discussionAnsweredCond is the EXISTS predicate behind Discussion.Answered
// and the ?answered= filter: a discussion is answered when one of its
// threads carries is_answer = 1. The schema's partial unique index keeps
// that at most one row per discussion.
const discussionAnsweredCond = `EXISTS (
	SELECT 1 FROM discussion_threads dt
	WHERE dt.discussion_id = d.id AND dt.is_answer = 1
)`

// GetDiscussion returns one discussion with Category and Answered, or
// ErrNotFound. The lookup is global — callers that need workspace scoping
// compare Discussion.WorkspaceID against their context.
func (s *Store) GetDiscussion(ctx context.Context, id int64) (*Discussion, error) {
	d, err := scanDiscussion(s.db.QueryRowContext(ctx,
		`SELECT d.id, d.workspace_id, d.title, d.body, d.created_at, d.updated_at,
			c.id, c.name, c.slug, c.description, c.format,
			u.id, u.name, u.created_at,
			`+discussionAnsweredCond+`
			FROM discussions d
			JOIN discussion_categories c ON c.id = d.category_id
			LEFT JOIN users u ON u.id = d.author_id
			WHERE d.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.attachDiscussionLabels(ctx, []*Discussion{d}); err != nil {
		return nil, err
	}
	return d, nil
}

// CreateDiscussion inserts a discussion in workspace wsID and returns
// its new ID (also the discussion number; IDs are never reused, and are
// independent of issue numbers). authorID is the posting user's ID, or 0
// for an anonymous post. Returns ErrInvalidCategory when the category
// does not exist.
func (s *Store) CreateDiscussion(ctx context.Context, wsID int64, in DiscussionInput, authorID int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := requireCategory(ctx, tx, in.CategoryID); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO discussions(workspace_id, category_id, title, body, created_at, updated_at, author_id) VALUES(?, ?, ?, ?, ?, ?, ?)",
		wsID, in.CategoryID, in.Title, in.Body, now, now, nullID(authorID))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := setDiscussionLabels(ctx, tx, id, wsID, in.LabelIDs); err != nil {
		return 0, err
	}
	if err := s.commitTx(ctx, tx, events.Change{Topic: events.DiscussionsTopic, Kind: events.KindNew, Target: id}); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateDiscussion replaces title/body/category and bumps UpdatedAt.
// Returns ErrNotFound if the discussion does not exist, ErrInvalidCategory
// for an unknown category. Moving a discussion to a non-qa category clears
// any answer marks, since they are a qa-only concept and would otherwise
// stay stranded (list shows Answered, detail can no longer unmark).
func (s *Store) UpdateDiscussion(ctx context.Context, id int64, in DiscussionInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	format, err := requireCategory(ctx, tx, in.CategoryID)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		"UPDATE discussions SET title = ?, body = ?, category_id = ?, updated_at = ? WHERE id = ?",
		in.Title, in.Body, in.CategoryID, time.Now().UTC().Unix(), id)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	var wsID int64
	if err := tx.QueryRowContext(ctx, "SELECT workspace_id FROM discussions WHERE id = ?", id).Scan(&wsID); err != nil {
		return err
	}
	if err := setDiscussionLabels(ctx, tx, id, wsID, in.LabelIDs); err != nil {
		return err
	}
	changes := []events.Change{{Topic: events.DiscussionTopic(id), Kind: events.KindEdit, Target: id}}
	var cleared int64
	if format != CategoryFormatQA {
		res, err := tx.ExecContext(ctx,
			"UPDATE discussion_threads SET is_answer = 0 WHERE discussion_id = ? AND is_answer = 1",
			id)
		if err != nil {
			return err
		}
		cleared, _ = res.RowsAffected()
		if cleared > 0 {
			// Marks vanished from threads the article swap does not touch —
			// tell watchers to rebuild the thread list, not just the card.
			changes = append(changes, events.Change{Topic: events.DiscussionTopic(id), Kind: events.KindResync, Target: id})
		}
	}
	return s.commitTx(ctx, tx, changes...)
}

// requireCategory verifies the category exists inside tx and returns its
// format (open|qa).
func requireCategory(ctx context.Context, tx *sql.Tx, categoryID int64) (string, error) {
	var format string
	err := tx.QueryRowContext(ctx,
		"SELECT format FROM discussion_categories WHERE id = ?", categoryID).Scan(&format)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidCategory
	}
	return format, err
}

// attachDiscussionLabels populates Labels on each discussion with a
// single grouped query — the issue counterpart is attachLabels.
func (s *Store) attachDiscussionLabels(ctx context.Context, discussions []*Discussion) error {
	if len(discussions) == 0 {
		return nil
	}
	byID := make(map[int64]*Discussion, len(discussions))
	ph := make([]string, len(discussions))
	args := make([]any, len(discussions))
	for i, d := range discussions {
		byID[d.ID] = d
		ph[i] = "?"
		args[i] = d.ID
	}
	q := `SELECT dl.discussion_id, l.id, l.name, l.color, l.kind, COALESCE(l.workspace_id, 0), l.archived
		FROM discussion_labels dl JOIN labels l ON l.id = dl.label_id
		WHERE dl.discussion_id IN (` + strings.Join(ph, ",") + `) ORDER BY l.name`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var discussionID int64
		var l Label
		if err := rows.Scan(&discussionID, &l.ID, &l.Name, &l.Color, &l.Kind, &l.WorkspaceID, &l.Archived); err != nil {
			return err
		}
		if d := byID[discussionID]; d != nil {
			d.Labels = append(d.Labels, l)
		}
	}
	return rows.Err()
}

// setDiscussionLabels replaces the label set of a discussion inside tx,
// mirroring setIssueLabels: unknown ids, cross-kind ids, out-of-workspace
// ids and archived labels yield no row and are silently skipped.
// Attachments to archived labels survive the replace for the same reason.
func setDiscussionLabels(ctx context.Context, tx *sql.Tx, discussionID, wsID int64, labelIDs []int64) error {
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM discussion_labels WHERE discussion_id = ? AND label_id NOT IN (SELECT id FROM labels WHERE archived = 1)",
		discussionID); err != nil {
		return err
	}
	for _, lid := range labelIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO discussion_labels(discussion_id, label_id)
				SELECT ?, id FROM labels WHERE id = ? AND kind = '`+KindLabelDiscussion+`' AND archived = 0
				AND (workspace_id IS NULL OR workspace_id = ?)`,
			discussionID, lid, wsID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDiscussion removes the discussion; its comments and replies go via
// ON DELETE CASCADE. Returns ErrNotFound if the discussion does not exist.
func (s *Store) DeleteDiscussion(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, "DELETE FROM discussions WHERE id = ?", id)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.DiscussionTopic(id), Kind: events.KindDelete, Target: id},
		events.Change{Topic: events.DiscussionsTopic, Kind: events.KindDelete, Target: id})
}

// ListDiscussionThreads returns the threads of a discussion in
// chronological order, each with its comments (chronological) attached.
func (s *Store) ListDiscussionThreads(ctx context.Context, discussionID int64) ([]*DiscussionThread, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.discussion_id, t.is_answer, t.body, t.created_at,
			t.edited_at, t.hidden_at, t.hidden_reason,
			u.id, u.name, u.created_at
			FROM discussion_threads t LEFT JOIN users u ON u.id = t.author_id
			WHERE t.discussion_id = ? ORDER BY t.created_at ASC, t.id ASC`,
		discussionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*DiscussionThread
	byID := map[int64]*DiscussionThread{}
	for rows.Next() {
		t, err := scanDiscussionThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		byID[t.ID] = t
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	replies, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.thread_id, c.body, c.created_at,
			c.edited_at, c.hidden_at, c.hidden_reason,
			u.id, u.name, u.created_at
			FROM discussion_comments c LEFT JOIN users u ON u.id = c.author_id
			WHERE c.thread_id IN (SELECT id FROM discussion_threads WHERE discussion_id = ?)
			ORDER BY c.created_at ASC, c.id ASC`,
		discussionID)
	if err != nil {
		return nil, err
	}
	defer replies.Close()
	for replies.Next() {
		c, err := scanDiscussionComment(replies)
		if err != nil {
			return nil, err
		}
		// A thread created between the two queries is skipped — the
		// listing is a snapshot of what the first query saw.
		if t, ok := byID[c.ThreadID]; ok {
			t.Comments = append(t.Comments, c)
		}
	}
	return out, replies.Err()
}

// CreateDiscussionThread opens a thread on the discussion — the post a
// "comment" form submits. Returns ErrNotFound if the discussion is
// missing. authorID is the posting user's ID, or 0 for an anonymous post.
func (s *Store) CreateDiscussionThread(ctx context.Context, discussionID int64, body string, authorID int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM discussions WHERE id = ?", discussionID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO discussion_threads(discussion_id, body, created_at, author_id) VALUES(?, ?, ?, ?)",
		discussionID, body, now, nullID(authorID))
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE discussions SET updated_at = ? WHERE id = ?", now, discussionID); err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := s.commitTx(ctx, tx, events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindThread, Target: id}); err != nil {
		return 0, err
	}
	return id, nil
}

// CreateDiscussionComment appends a reply to a thread of the discussion.
// Returns ErrNotFound if the discussion is missing and ErrInvalidThread
// when the thread does not belong to this discussion. authorID is the
// posting user's ID, or 0 for an anonymous post.
func (s *Store) CreateDiscussionComment(ctx context.Context, discussionID, threadID int64, body string, authorID int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM discussions WHERE id = ?", discussionID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var tid int64
	err = tx.QueryRowContext(ctx,
		"SELECT id FROM discussion_threads WHERE id = ? AND discussion_id = ?",
		threadID, discussionID).Scan(&tid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrInvalidThread
	}
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Unix()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO discussion_comments(thread_id, body, created_at, author_id) VALUES(?, ?, ?, ?)",
		threadID, body, now, nullID(authorID))
	if err != nil {
		return 0, err
	}
	// A new thread or reply is activity on the discussion, so bump its
	// updated_at with the comment's timestamp — the same rule issues use.
	if _, err := tx.ExecContext(ctx,
		"UPDATE discussions SET updated_at = ? WHERE id = ?", now, discussionID); err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := s.commitTx(ctx, tx, events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindReply, Target: id, Parent: threadID}); err != nil {
		return 0, err
	}
	return id, nil
}

// SetThreadAnswer marks (answer=true) or unmarks a thread as the
// discussion's answer, then bumps the discussion's updated_at — the
// mark is activity, the same rule comment posting follows. Only qa-format
// categories support answers. Returns ErrNotFound when the discussion or
// thread is missing (a reply's id also lands here — comments are never
// answerable), and ErrNotAnswerable when the category is not qa. Marking
// a second thread moves the mark; unmarking an unmarked thread is a no-op.
func (s *Store) SetThreadAnswer(ctx context.Context, discussionID, threadID int64, answer bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var format string
	err = tx.QueryRowContext(ctx,
		`SELECT c.format FROM discussions d
		 JOIN discussion_categories c ON c.id = d.category_id
		 WHERE d.id = ?`, discussionID).Scan(&format)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var isThread int
	err = tx.QueryRowContext(ctx,
		"SELECT 1 FROM discussion_threads WHERE id = ? AND discussion_id = ?",
		threadID, discussionID).Scan(&isThread)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if answer && format != CategoryFormatQA {
		return ErrNotAnswerable
	}

	// The change event reports the previous mark holder so the client can
	// re-render both <li>s (the mark moved). Read it before the update
	// clears it; marking a fresh thread leaves prev = 0.
	var prev int64
	err = tx.QueryRowContext(ctx,
		"SELECT id FROM discussion_threads WHERE discussion_id = ? AND is_answer = 1",
		discussionID).Scan(&prev)
	if errors.Is(err, sql.ErrNoRows) {
		prev = 0
	} else if err != nil {
		return err
	}

	if answer {
		// Move the mark: clear the previous answer (at most one row
		// thanks to the partial unique index), then set the new one.
		if _, err := tx.ExecContext(ctx,
			"UPDATE discussion_threads SET is_answer = 0 WHERE discussion_id = ? AND is_answer = 1",
			discussionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE discussion_threads SET is_answer = 1 WHERE id = ?",
			threadID); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			"UPDATE discussion_threads SET is_answer = 0 WHERE id = ?",
			threadID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE discussions SET updated_at = ? WHERE id = ?",
		time.Now().UTC().Unix(), discussionID); err != nil {
		return err
	}
	return s.commitTx(ctx, tx, events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindAnswer, Target: threadID, Parent: prev})
}

func scanDiscussion(s scanner) (*Discussion, error) {
	var d Discussion
	var c, u int64
	var aid sql.NullInt64
	var aname sql.NullString
	var acreated sql.NullInt64
	var answered int64
	if err := s.Scan(&d.ID, &d.WorkspaceID, &d.Title, &d.Body, &c, &u,
		&d.Category.ID, &d.Category.Name, &d.Category.Slug, &d.Category.Description, &d.Category.Format,
		&aid, &aname, &acreated, &answered); err != nil {
		return nil, err
	}
	d.CreatedAt = time.Unix(c, 0).UTC()
	d.UpdatedAt = time.Unix(u, 0).UTC()
	d.Author = scanUser(aid, aname, acreated)
	d.Answered = answered != 0
	return &d, nil
}

func scanDiscussionThread(s scanner) (*DiscussionThread, error) {
	var t DiscussionThread
	var ts int64
	var ed, hid sql.NullInt64
	var reason sql.NullString
	var aid sql.NullInt64
	var aname sql.NullString
	var acreated sql.NullInt64
	if err := s.Scan(&t.ID, &t.DiscussionID, &t.IsAnswer, &t.Body, &ts,
		&ed, &hid, &reason, &aid, &aname, &acreated); err != nil {
		return nil, err
	}
	t.CreatedAt = time.Unix(ts, 0).UTC()
	t.Moderation = scanModeration(ed, hid, reason)
	t.Author = scanUser(aid, aname, acreated)
	return &t, nil
}

func scanDiscussionComment(s scanner) (*DiscussionComment, error) {
	var c DiscussionComment
	var ts int64
	var ed, hid sql.NullInt64
	var reason sql.NullString
	var aid sql.NullInt64
	var aname sql.NullString
	var acreated sql.NullInt64
	if err := s.Scan(&c.ID, &c.ThreadID, &c.Body, &ts,
		&ed, &hid, &reason, &aid, &aname, &acreated); err != nil {
		return nil, err
	}
	c.CreatedAt = time.Unix(ts, 0).UTC()
	c.Moderation = scanModeration(ed, hid, reason)
	c.Author = scanUser(aid, aname, acreated)
	return &c, nil
}
