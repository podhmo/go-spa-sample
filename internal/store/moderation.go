// Comment moderation — the edit/delete/hide operations shared by the
// three comment-like rows (issue comments, discussion threads, replies).
// Semantics: an edit stamps edited_at (only when the body changed, the
// same conditional bump SetIssueStatus uses for updated_at); hide carries
// a required reason and keeps the body in place; delete is a hard DELETE
// (replies under a deleted thread go with it via ON DELETE CASCADE).
// None of them bumps the parent's updated_at: a moderation touch is not
// new activity, the same rule SetIssueStatus already follows.
package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

// Hide reasons — the fixed set a minimizer picks from. The values are
// part of the schema CHECK constraint, the HTML <select>, the API and
// the zatu --reason flag, so they live here once (user request; mirrors
// GitHub's minimize reasons plus outdated/low-quality).
const (
	HideReasonSpam       = "spam"
	HideReasonAbuse      = "abuse"
	HideReasonOffTopic   = "off-topic"
	HideReasonOutdated   = "outdated"
	HideReasonDuplicate  = "duplicate"
	HideReasonResolved   = "resolved"
	HideReasonLowQuality = "low-quality"
)

// HideReasons lists every valid reason in display order — the web form
// iterates it, the store and the API validate against it.
var HideReasons = []string{
	HideReasonSpam,
	HideReasonAbuse,
	HideReasonOffTopic,
	HideReasonOutdated,
	HideReasonDuplicate,
	HideReasonResolved,
	HideReasonLowQuality,
}

// ErrInvalidHideReason reports a Set*Hidden call carrying a reason
// outside HideReasons (a hand-crafted request — the form only offers
// valid ones). Handlers map it to a 400/422.
var ErrInvalidHideReason = errors.New("store: invalid hide reason")

// ValidHideReason reports whether s names a hide reason; "" is invalid —
// a hidden post must carry why it was minimized.
func ValidHideReason(s string) bool {
	return slices.Contains(HideReasons, s)
}

// Moderation carries the edit/hide state shared by the three post types.
// It stays embedded (anonymous) so templates and DTOs reach .Edited() /
// .Hidden / .HiddenReason identically on Comment, DiscussionThread and
// DiscussionComment (templates spell it .C.Moderation.Hidden — minigo
// can't resolve promoted members, see docs/ja/pitfalls.md). The
// hidden_at timestamp lives only in the schema —
// the domain keeps the flag; the reason string is the visible audit.
type Moderation struct {
	EditedAt     time.Time // zero = never edited
	Hidden       bool
	HiddenReason string // a HideReason* value, valid only when Hidden
}

// Edited reports whether the body changed after creation.
func (m Moderation) Edited() bool { return !m.EditedAt.IsZero() }

// scanModeration maps the shared (edited_at, hidden_at, hidden_reason)
// column triple — appended in that order to every post SELECT — to the
// embedded Moderation.
func scanModeration(edited, hidden sql.NullInt64, reason sql.NullString) Moderation {
	var m Moderation
	if edited.Valid {
		m.EditedAt = time.Unix(edited.Int64, 0).UTC()
	}
	m.Hidden = hidden.Valid
	m.HiddenReason = reason.String
	return m
}

// UpdateComment replaces a comment's body, stamping edited_at when the
// text changed. ErrNotFound when the comment is missing or belongs to a
// different issue.
func (s *Store) UpdateComment(ctx context.Context, issueID, commentID int64, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE comments SET body = ?,
			edited_at = CASE WHEN body != ? THEN ? ELSE edited_at END
			WHERE id = ? AND issue_id = ?`,
		body, body, time.Now().UTC().Unix(), commentID, issueID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.IssueTopic(issueID), Kind: events.KindComment, Target: commentID})
}

// DeleteComment removes a comment permanently. ErrNotFound when missing
// or belonging to a different issue.
func (s *Store) DeleteComment(ctx context.Context, issueID, commentID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		"DELETE FROM comments WHERE id = ? AND issue_id = ?", commentID, issueID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.IssueTopic(issueID), Kind: events.KindRemove, Target: commentID})
}

// SetCommentHidden minimizes a comment with a required reason, or
// restores it when reason is "". ErrInvalidHideReason for a reason
// outside HideReasons; ErrNotFound for a missing/mismatched comment.
func (s *Store) SetCommentHidden(ctx context.Context, issueID, commentID int64, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := setHidden(ctx, tx, "comments", "issue_id", issueID, commentID, reason)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.IssueTopic(issueID), Kind: events.KindComment, Target: commentID})
}

// UpdateDiscussionThread replaces a thread's body (the discussion's
// opening post), stamping edited_at on a real change. ErrNotFound when
// the thread is missing or belongs to a different discussion.
func (s *Store) UpdateDiscussionThread(ctx context.Context, discussionID, threadID int64, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE discussion_threads SET body = ?,
			edited_at = CASE WHEN body != ? THEN ? ELSE edited_at END
			WHERE id = ? AND discussion_id = ?`,
		body, body, time.Now().UTC().Unix(), threadID, discussionID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindThread, Target: threadID})
}

// DeleteDiscussionThread removes a thread and its replies (CASCADE); a
// marked answer simply leaves the discussion unanswered. ErrNotFound
// when the thread is missing or belongs to a different discussion.
func (s *Store) DeleteDiscussionThread(ctx context.Context, discussionID, threadID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		"DELETE FROM discussion_threads WHERE id = ? AND discussion_id = ?", threadID, discussionID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindRemove, Target: threadID})
}

// SetDiscussionThreadHidden minimizes a thread with a required reason,
// or restores it when reason is "". Replies stay visible — they are
// other posts, not the hidden body. ErrInvalidHideReason/ErrNotFound.
func (s *Store) SetDiscussionThreadHidden(ctx context.Context, discussionID, threadID int64, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := setHidden(ctx, tx, "discussion_threads", "discussion_id", discussionID, threadID, reason)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindThread, Target: threadID})
}

// UpdateDiscussionComment replaces a reply's body, stamping edited_at on
// a real change. ErrNotFound when the reply is missing or the
// (discussion, thread, reply) chain does not line up.
func (s *Store) UpdateDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE discussion_comments SET body = ?,
			edited_at = CASE WHEN body != ? THEN ? ELSE edited_at END
			WHERE id = ? AND thread_id = ? AND EXISTS (
				SELECT 1 FROM discussion_threads t
				WHERE t.id = discussion_comments.thread_id AND t.discussion_id = ?)`,
		body, body, time.Now().UTC().Unix(), commentID, threadID, discussionID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindReply, Target: commentID, Parent: threadID})
}

// DeleteDiscussionComment removes a reply. ErrNotFound when the
// (discussion, thread, reply) chain does not resolve.
func (s *Store) DeleteDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`DELETE FROM discussion_comments
			WHERE id = ? AND thread_id = ? AND EXISTS (
				SELECT 1 FROM discussion_threads t
				WHERE t.id = discussion_comments.thread_id AND t.discussion_id = ?)`,
		commentID, threadID, discussionID)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindRemove, Target: commentID, Parent: threadID})
}

// SetDiscussionCommentHidden minimizes a reply with a required reason,
// or restores it when reason is "". ErrInvalidHideReason/ErrNotFound.
func (s *Store) SetDiscussionCommentHidden(ctx context.Context, discussionID, threadID, commentID int64, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := setHiddenScoped(ctx, tx, commentID, threadID, discussionID, reason)
	if err != nil {
		return err
	}
	if err := checkAffected(res); err != nil {
		return err
	}
	return s.commitTx(ctx, tx,
		events.Change{Topic: events.DiscussionTopic(discussionID), Kind: events.KindReply, Target: commentID, Parent: threadID})
}

// setHidden writes the hidden_at/hidden_reason pair on one row of table
// filtered by its parent column. reason "" restores (clears both);
// anything else must pass ValidHideReason first.
func setHidden(ctx context.Context, tx *sql.Tx, table, parentCol string, parentID, id int64, reason string) (sql.Result, error) {
	if reason == "" {
		return tx.ExecContext(ctx,
			"UPDATE "+table+" SET hidden_at = NULL, hidden_reason = NULL WHERE id = ? AND "+parentCol+" = ?",
			id, parentID)
	}
	if !ValidHideReason(reason) {
		return nil, ErrInvalidHideReason
	}
	return tx.ExecContext(ctx,
		"UPDATE "+table+" SET hidden_at = ?, hidden_reason = ? WHERE id = ? AND "+parentCol+" = ?",
		time.Now().UTC().Unix(), reason, id, parentID)
}

// setHiddenScoped is setHidden for discussion_comments, whose parent
// chain (discussion → thread → reply) needs the EXISTS probe instead of
// a flat parent column.
func setHiddenScoped(ctx context.Context, tx *sql.Tx, commentID, threadID, discussionID int64, reason string) (sql.Result, error) {
	if reason != "" && !ValidHideReason(reason) {
		return nil, ErrInvalidHideReason
	}
	var res sql.Result
	var err error
	if reason == "" {
		res, err = tx.ExecContext(ctx,
			`UPDATE discussion_comments SET hidden_at = NULL, hidden_reason = NULL
				WHERE id = ? AND thread_id = ? AND EXISTS (
					SELECT 1 FROM discussion_threads t
					WHERE t.id = discussion_comments.thread_id AND t.discussion_id = ?)`,
			commentID, threadID, discussionID)
	} else {
		res, err = tx.ExecContext(ctx,
			`UPDATE discussion_comments SET hidden_at = ?, hidden_reason = ?
				WHERE id = ? AND thread_id = ? AND EXISTS (
					SELECT 1 FROM discussion_threads t
					WHERE t.id = discussion_comments.thread_id AND t.discussion_id = ?)`,
			time.Now().UTC().Unix(), reason, commentID, threadID, discussionID)
	}
	return res, err
}
