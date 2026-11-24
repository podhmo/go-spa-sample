package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

// findCommentID returns one comment of the issue by id, or nil.
func findCommentID(cs []*Comment, id int64) *Comment {
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func TestUpdateComment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	issueID, err := s.CreateIssue(ctx, 1, IssueInput{Title: "x"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	cid, err := s.CreateComment(ctx, issueID, "original", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	other, err := s.CreateIssue(ctx, 1, IssueInput{Title: "other"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	backdate(t, s, issueID, time.Hour)
	before, err := s.GetIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	// An unchanged body must not stamp edited_at.
	if err := s.UpdateComment(ctx, issueID, cid, "original"); err != nil {
		t.Fatalf("UpdateComment(no-op): %v", err)
	}
	cs, err := s.ListComments(ctx, issueID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if c := findCommentID(cs, cid); c.Edited() {
		t.Fatalf("no-op edit stamped edited_at: %+v", c)
	}

	if err := s.UpdateComment(ctx, issueID, cid, "fixed"); err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	cs, err = s.ListComments(ctx, issueID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	c := findCommentID(cs, cid)
	if c == nil || c.Body != "fixed" || !c.Edited() {
		t.Fatalf("edited comment = %+v", c)
	}
	if c.EditedAt.Before(c.CreatedAt) {
		t.Fatalf("edited_at %v before created_at %v", c.EditedAt, c.CreatedAt)
	}

	// A moderation touch is not activity on the issue.
	after, err := s.GetIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("edit bumped updated_at: %v -> %v", before.UpdatedAt, after.UpdatedAt)
	}

	// Scope and existence.
	if err := s.UpdateComment(ctx, other, cid, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-issue update = %v, want ErrNotFound", err)
	}
	if err := s.UpdateComment(ctx, issueID, 9999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing comment = %v, want ErrNotFound", err)
	}
	if err := s.UpdateComment(ctx, 9999, cid, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing issue = %v, want ErrNotFound", err)
	}
}

func TestDeleteComment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	issueID, err := s.CreateIssue(ctx, 1, IssueInput{Title: "x"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	cid, err := s.CreateComment(ctx, issueID, "bye", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	keep, err := s.CreateComment(ctx, issueID, "keep", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if err := s.DeleteComment(ctx, issueID, cid); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	cs, err := s.ListComments(ctx, issueID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(cs) != 1 || cs[0].ID != keep {
		t.Fatalf("comments = %+v, want just %d", cs, keep)
	}
	if err := s.DeleteComment(ctx, issueID, cid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
	if err := s.DeleteComment(ctx, issueID, keep); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
}

func TestCommentHideUnhide(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	issueID, err := s.CreateIssue(ctx, 1, IssueInput{Title: "x"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	cid, err := s.CreateComment(ctx, issueID, "spam me", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if err := s.SetCommentHidden(ctx, issueID, cid, "bogus"); !errors.Is(err, ErrInvalidHideReason) {
		t.Fatalf("bad reason = %v, want ErrInvalidHideReason", err)
	}
	// Every declared reason round-trips.
	if err := s.SetCommentHidden(ctx, issueID, cid, HideReasonSpam); err != nil {
		t.Fatalf("hide: %v", err)
	}
	cs, err := s.ListComments(ctx, issueID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	c := findCommentID(cs, cid)
	if !c.Hidden || c.HiddenReason != HideReasonSpam {
		t.Fatalf("hidden comment = %+v", c)
	}
	// The body is still there — hidden is minimized, not erased.
	if c.Body != "spam me" {
		t.Fatalf("body = %q", c.Body)
	}
	var hid sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT hidden_at FROM comments WHERE id = ?", cid).Scan(&hid); err != nil {
		t.Fatalf("hidden_at: %v", err)
	}
	if !hid.Valid {
		t.Fatal("hidden_at not stamped")
	}

	// Re-hiding switches the reason and restamps the timestamp.
	if err := s.SetCommentHidden(ctx, issueID, cid, HideReasonDuplicate); err != nil {
		t.Fatalf("re-hide: %v", err)
	}
	cs, _ = s.ListComments(ctx, issueID)
	if c = findCommentID(cs, cid); !c.Hidden || c.HiddenReason != HideReasonDuplicate {
		t.Fatalf("re-hidden comment = %+v", c)
	}

	if err := s.SetCommentHidden(ctx, issueID, cid, ""); err != nil {
		t.Fatalf("unhide: %v", err)
	}
	cs, _ = s.ListComments(ctx, issueID)
	if c = findCommentID(cs, cid); c.Hidden || c.HiddenReason != "" {
		t.Fatalf("unhidden comment = %+v", c)
	}
	var hid2 sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT hidden_at FROM comments WHERE id = ?", cid).Scan(&hid2); err != nil {
		t.Fatalf("hidden_at: %v", err)
	}
	if hid2.Valid {
		t.Fatal("unhide left hidden_at set")
	}

	if err := s.SetCommentHidden(ctx, issueID, 9999, HideReasonSpam); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing comment = %v, want ErrNotFound", err)
	}
	other, err := s.CreateIssue(ctx, 1, IssueInput{Title: "y"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := s.SetCommentHidden(ctx, other, cid, HideReasonSpam); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-issue hide = %v, want ErrNotFound", err)
	}
}

// TestThreadModeration covers update/hide/delete on discussion threads,
// including the answer-mark bookkeeping when the deleted thread was the
// answer.
func TestThreadModeration(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	qa := mustCategory(t, s, "Q&A", "qa", CategoryFormatQA)
	did := mustDiscussion(t, s, qa, "q")

	tid, err := s.CreateDiscussionThread(ctx, did, "the answer", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionThread: %v", err)
	}
	rid, err := s.CreateDiscussionComment(ctx, did, tid, "thanks", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionComment: %v", err)
	}
	if err := s.SetThreadAnswer(ctx, did, tid, true); err != nil {
		t.Fatalf("SetThreadAnswer: %v", err)
	}
	other := mustDiscussion(t, s, qa, "other q")

	if err := s.UpdateDiscussionThread(ctx, did, tid, "edited answer"); err != nil {
		t.Fatalf("UpdateDiscussionThread: %v", err)
	}
	ts, err := s.ListDiscussionThreads(ctx, did)
	if err != nil {
		t.Fatalf("ListDiscussionThreads: %v", err)
	}
	if len(ts) != 1 || ts[0].Body != "edited answer" || !ts[0].Edited() || !ts[0].IsAnswer {
		t.Fatalf("edited thread = %+v", ts)
	}

	if err := s.SetDiscussionThreadHidden(ctx, did, tid, HideReasonResolved); err != nil {
		t.Fatalf("hide thread: %v", err)
	}
	ts, _ = s.ListDiscussionThreads(ctx, did)
	if !ts[0].Hidden || ts[0].HiddenReason != HideReasonResolved {
		t.Fatalf("hidden thread = %+v", ts[0])
	}
	// Replies stay listed under a hidden thread.
	if len(ts[0].Comments) != 1 || ts[0].Comments[0].ID != rid {
		t.Fatalf("replies of hidden thread = %+v", ts[0].Comments)
	}
	if err := s.SetDiscussionThreadHidden(ctx, did, tid, ""); err != nil {
		t.Fatalf("unhide thread: %v", err)
	}

	// Deleting the marked thread clears the discussion's answered state
	// and takes its replies along via CASCADE.
	if err := s.DeleteDiscussionThread(ctx, did, tid); err != nil {
		t.Fatalf("DeleteDiscussionThread: %v", err)
	}
	ts, _ = s.ListDiscussionThreads(ctx, did)
	if len(ts) != 0 {
		t.Fatalf("threads = %+v, want empty", ts)
	}
	d, err := s.GetDiscussion(ctx, did)
	if err != nil {
		t.Fatalf("GetDiscussion: %v", err)
	}
	if d.Answered {
		t.Fatal("discussion still answered after deleting the answer thread")
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM discussion_comments WHERE id = ?", rid).Scan(&n); err != nil {
		t.Fatalf("count reply: %v", err)
	}
	if n != 0 {
		t.Fatalf("reply %d survived its thread's deletion", rid)
	}

	// Scope checks: a thread id under a different discussion is not found.
	tid2, err := s.CreateDiscussionThread(ctx, did, "mine", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionThread: %v", err)
	}
	if err := s.UpdateDiscussionThread(ctx, other, tid2, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-discussion update = %v, want ErrNotFound", err)
	}
	if err := s.DeleteDiscussionThread(ctx, other, tid2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-discussion delete = %v, want ErrNotFound", err)
	}
	if err := s.SetDiscussionThreadHidden(ctx, other, tid2, HideReasonSpam); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-discussion hide = %v, want ErrNotFound", err)
	}
}

// TestReplyModeration covers update/hide/delete on discussion replies
// and the (discussion, thread, reply) scoping chain.
func TestReplyModeration(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	did := mustDiscussion(t, s, cat, "d")
	other := mustDiscussion(t, s, cat, "d2")

	tid, err := s.CreateDiscussionThread(ctx, did, "post", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionThread: %v", err)
	}
	otherTid, err := s.CreateDiscussionThread(ctx, other, "other post", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionThread: %v", err)
	}
	rid, err := s.CreateDiscussionComment(ctx, did, tid, "reply", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionComment: %v", err)
	}

	if err := s.UpdateDiscussionComment(ctx, did, tid, rid, "edited"); err != nil {
		t.Fatalf("UpdateDiscussionComment: %v", err)
	}
	if err := s.SetDiscussionCommentHidden(ctx, did, tid, rid, HideReasonOffTopic); err != nil {
		t.Fatalf("hide reply: %v", err)
	}
	ts, err := s.ListDiscussionThreads(ctx, did)
	if err != nil {
		t.Fatalf("ListDiscussionThreads: %v", err)
	}
	r := ts[0].Comments[0]
	if r.Body != "edited" || !r.Edited() || !r.Hidden || r.HiddenReason != HideReasonOffTopic {
		t.Fatalf("moderated reply = %+v", r)
	}
	if err := s.SetDiscussionCommentHidden(ctx, did, tid, rid, ""); err != nil {
		t.Fatalf("unhide reply: %v", err)
	}

	// The reply must resolve under its own thread of its own discussion.
	if err := s.UpdateDiscussionComment(ctx, did, otherTid, rid, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong thread = %v, want ErrNotFound", err)
	}
	if err := s.UpdateDiscussionComment(ctx, other, tid, rid, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong discussion = %v, want ErrNotFound", err)
	}
	if err := s.SetDiscussionCommentHidden(ctx, did, otherTid, rid, HideReasonSpam); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong thread hide = %v, want ErrNotFound", err)
	}
	if err := s.DeleteDiscussionComment(ctx, did, otherTid, rid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong thread delete = %v, want ErrNotFound", err)
	}

	if err := s.DeleteDiscussionComment(ctx, did, tid, rid); err != nil {
		t.Fatalf("DeleteDiscussionComment: %v", err)
	}
	ts, _ = s.ListDiscussionThreads(ctx, did)
	if len(ts[0].Comments) != 0 {
		t.Fatalf("comments = %+v, want empty", ts[0].Comments)
	}
	if err := s.DeleteDiscussionComment(ctx, did, tid, rid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

// TestModerationEvents checks the change hints each mutator commits:
// edits and hides ride the same kind as a create (subscribers upsert the
// row), deletes emit remove.
func TestModerationEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	issueID, err := s.CreateIssue(ctx, 1, IssueInput{Title: "x"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	cid, err := s.CreateComment(ctx, issueID, "c", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	cat := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	did := mustDiscussion(t, s, cat, "d")
	tid, err := s.CreateDiscussionThread(ctx, did, "t", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionThread: %v", err)
	}
	rid, err := s.CreateDiscussionComment(ctx, did, tid, "r", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionComment: %v", err)
	}
	seq, err := s.LatestChangeSeq(ctx)
	if err != nil {
		t.Fatalf("LatestChangeSeq: %v", err)
	}

	steps := []struct {
		name           string
		run            func() error
		topic, kind    string
		target, parent int64
	}{
		{"edit comment", func() error { return s.UpdateComment(ctx, issueID, cid, "c2") },
			events.IssueTopic(issueID), "comment", cid, 0},
		{"hide comment", func() error { return s.SetCommentHidden(ctx, issueID, cid, HideReasonSpam) },
			events.IssueTopic(issueID), "comment", cid, 0},
		{"delete comment", func() error { return s.DeleteComment(ctx, issueID, cid) },
			events.IssueTopic(issueID), "remove", cid, 0},
		{"edit thread", func() error { return s.UpdateDiscussionThread(ctx, did, tid, "t2") },
			events.DiscussionTopic(did), "thread", tid, 0},
		{"hide thread", func() error { return s.SetDiscussionThreadHidden(ctx, did, tid, HideReasonOutdated) },
			events.DiscussionTopic(did), "thread", tid, 0},
		{"edit reply", func() error { return s.UpdateDiscussionComment(ctx, did, tid, rid, "r2") },
			events.DiscussionTopic(did), "reply", rid, tid},
		{"hide reply", func() error { return s.SetDiscussionCommentHidden(ctx, did, tid, rid, HideReasonAbuse) },
			events.DiscussionTopic(did), "reply", rid, tid},
		{"delete reply", func() error { return s.DeleteDiscussionComment(ctx, did, tid, rid) },
			events.DiscussionTopic(did), "remove", rid, tid},
		{"delete thread", func() error { return s.DeleteDiscussionThread(ctx, did, tid) },
			events.DiscussionTopic(did), "remove", tid, 0},
	}
	for _, st := range steps {
		if err := st.run(); err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
	}
	changes, err := s.ListChanges(ctx, seq)
	if err != nil {
		t.Fatalf("ListChanges: %v", err)
	}
	if len(changes) != len(steps) {
		t.Fatalf("got %d changes, want %d", len(changes), len(steps))
	}
	for i, st := range steps {
		c := changes[i]
		if c.Topic != st.topic || c.Kind != st.kind || c.Target != st.target || c.Parent != st.parent {
			t.Fatalf("change[%d] (%s) = %+v", i, st.name, c)
		}
	}
}

// TestMigrateV4AddsColumns drives the upgrade path of the guarded ALTERs:
// a pre-moderation table (v3 shape) gains the columns on Open.
func TestMigrateV4AddsColumns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v3.db")

	// Build a database shaped like a v3 one: the moderation columns are
	// absent and user_version is stamped 3. Running migrate() then runs
	// only migrateV4.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE comments (id INTEGER PRIMARY KEY, issue_id INTEGER, body TEXT, created_at INTEGER);
		CREATE TABLE discussion_threads (id INTEGER PRIMARY KEY, discussion_id INTEGER, body TEXT, created_at INTEGER);
		CREATE TABLE discussion_comments (id INTEGER PRIMARY KEY, thread_id INTEGER, body TEXT, created_at INTEGER);
		INSERT INTO comments VALUES(1, 1, 'old', 0);
		PRAGMA user_version = 3`); err != nil {
		t.Fatalf("build v3 db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// migrate() alone: Open would create the full schema first.
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer raw.Close()
	if err := migrate(ctx, raw); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var version int
	if err := raw.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("version = %d, want %d", version, schemaVersion)
	}
	for _, table := range []string{"comments", "discussion_threads", "discussion_comments"} {
		var n int
		if err := raw.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pragma_table_info('"+table+"') WHERE name IN ('edited_at','hidden_at','hidden_reason')").Scan(&n); err != nil {
			t.Fatalf("probe %s: %v", table, err)
		}
		if n != 3 {
			t.Fatalf("%s gained %d moderation columns, want 3", table, n)
		}
	}
	// Existing rows keep working — NULLs read back as unedited/visible.
	var hid sql.NullInt64
	if err := raw.QueryRowContext(ctx, "SELECT hidden_at FROM comments WHERE id = 1").Scan(&hid); err != nil {
		t.Fatalf("select: %v", err)
	}
	if hid.Valid {
		t.Fatal("legacy row reads hidden")
	}
	// The CHECK lands on migrated columns too.
	if _, err := raw.ExecContext(ctx,
		"UPDATE comments SET hidden_reason = 'bogus' WHERE id = 1"); err == nil {
		t.Fatal("CHECK constraint missing on migrated column")
	}
}
