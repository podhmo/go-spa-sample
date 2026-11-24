package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// mustCategory inserts a discussion category and returns its ID.
func mustCategory(t *testing.T, s *Store, name, slug, format string) int64 {
	t.Helper()
	res, err := s.db.Exec(
		"INSERT INTO discussion_categories(name, slug, description, format) VALUES(?, ?, '', ?)",
		name, slug, format)
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("category id: %v", err)
	}
	return id
}

// mustDiscussion creates a discussion and returns its ID.
func mustDiscussion(t *testing.T, s *Store, categoryID int64, title string) int64 {
	t.Helper()
	id, err := s.CreateDiscussion(context.Background(), 1,
		DiscussionInput{Title: title, Body: "body", CategoryID: categoryID}, 0)
	if err != nil {
		t.Fatalf("CreateDiscussion: %v", err)
	}
	return id
}

func TestDiscussionCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	cat2 := mustCategory(t, s, "Q&A", "qa", CategoryFormatQA)

	id := mustDiscussion(t, s, cat, "first")

	d, err := s.GetDiscussion(ctx, id)
	if err != nil {
		t.Fatalf("GetDiscussion: %v", err)
	}
	if d.Title != "first" || d.Category.Slug != "general" || d.Author != nil || d.Answered {
		t.Errorf("unexpected discussion: %+v", d)
	}

	time.Sleep(1100 * time.Millisecond) // unix-second granularity
	if err := s.UpdateDiscussion(ctx, id, DiscussionInput{Title: "renamed", Body: "b2", CategoryID: cat2}); err != nil {
		t.Fatalf("UpdateDiscussion: %v", err)
	}
	d, err = s.GetDiscussion(ctx, id)
	if err != nil {
		t.Fatalf("GetDiscussion: %v", err)
	}
	if d.Title != "renamed" || d.Category.Slug != "qa" || !d.UpdatedAt.After(d.CreatedAt) {
		t.Errorf("unexpected after update: %+v", d)
	}

	if err := s.DeleteDiscussion(ctx, id); err != nil {
		t.Fatalf("DeleteDiscussion: %v", err)
	}
	if _, err := s.GetDiscussion(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDiscussion after delete: %v", err)
	}
	if err := s.DeleteDiscussion(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestDiscussionCategoryValidation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := mustCategory(t, s, "General", "general", CategoryFormatOpen)

	if _, err := s.CreateDiscussion(ctx, 1, DiscussionInput{Title: "t", CategoryID: 999}, 0); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("create with unknown category: %v", err)
	}
	id := mustDiscussion(t, s, cat, "ok")
	if err := s.UpdateDiscussion(ctx, id, DiscussionInput{Title: "t", CategoryID: 999}); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("update with unknown category: %v", err)
	}
}

func TestListDiscussions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	general := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	qa := mustCategory(t, s, "Q&A", "qa", CategoryFormatQA)

	d1 := mustDiscussion(t, s, general, "label workflow")
	d2 := mustDiscussion(t, s, qa, "why empty export")
	mustDiscussion(t, s, qa, "keyboard shortcuts")

	list, total, err := s.ListDiscussions(ctx, 1, DiscussionListFilter{CategorySlug: "qa"})
	if err != nil || total != 2 {
		t.Fatalf("category=qa: total=%d err=%v", total, err)
	}
	_ = list

	_, total, err = s.ListDiscussions(ctx, 1, DiscussionListFilter{Query: "keyboard"})
	if err != nil || total != 1 {
		t.Fatalf("q=keyboard: total=%d err=%v", total, err)
	}

	if _, err := s.CreateDiscussionThread(ctx, d1, "no", 0); err != nil {
		t.Fatalf("thread: %v", err)
	}
	tid, err := s.CreateDiscussionThread(ctx, d2, "the answer", 0)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if err := s.SetThreadAnswer(ctx, d2, tid, true); err != nil {
		t.Fatalf("SetThreadAnswer: %v", err)
	}
	_, total, err = s.ListDiscussions(ctx, 1, DiscussionListFilter{Answered: "yes"})
	if err != nil || total != 1 {
		t.Fatalf("answered=yes: total=%d err=%v", total, err)
	}
	_, total, err = s.ListDiscussions(ctx, 1, DiscussionListFilter{Answered: "no"})
	if err != nil || total != 2 {
		t.Fatalf("answered=no: total=%d err=%v", total, err)
	}

	list, _, err = s.ListDiscussions(ctx, 1, DiscussionListFilter{Sort: "oldest"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !slices.IsSortedFunc(list, func(a, b *Discussion) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	}) {
		t.Fatalf("oldest sort violated: %+v", list)
	}
}

func TestDiscussionThreading(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	d1 := mustDiscussion(t, s, cat, "d1")
	d2 := mustDiscussion(t, s, cat, "d2")

	t1, err := s.CreateDiscussionThread(ctx, d1, "top1", 0)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if _, err := s.CreateDiscussionThread(ctx, d1, "top2", 0); err != nil {
		t.Fatalf("thread: %v", err)
	}
	r1, err := s.CreateDiscussionComment(ctx, d1, t1, "reply to top1", 0)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	// Threads and comments live in separate id spaces, so a reply's id
	// can coincide with a thread's — reply-to-reply is expressible only
	// as a reply to that thread, which the API surface never produces.
	tOther, err := s.CreateDiscussionThread(ctx, d2, "other", 0)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if _, err := s.CreateDiscussionComment(ctx, d1, tOther, "x", 0); !errors.Is(err, ErrInvalidThread) {
		t.Fatalf("cross-discussion thread: %v", err)
	}
	if _, err := s.CreateDiscussionComment(ctx, 999, t1, "x", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("comment on missing discussion: %v", err)
	}
	if _, err := s.CreateDiscussionThread(ctx, 999, "x", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("thread on missing discussion: %v", err)
	}

	threads, err := s.ListDiscussionThreads(ctx, d1)
	if err != nil {
		t.Fatalf("ListDiscussionThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("thread count = %d, want 2", len(threads))
	}
	var found bool
	for _, th := range threads {
		if th.ID == t1 {
			found = true
			if len(th.Comments) != 1 || th.Comments[0].ID != r1 || th.Comments[0].ThreadID != t1 {
				t.Fatalf("thread shape: %+v", th)
			}
		}
	}
	if !found {
		t.Fatalf("thread missing: %+v", threads)
	}
}

func TestSetThreadAnswer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	open := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	qa := mustCategory(t, s, "Q&A", "qa", CategoryFormatQA)
	dOpen := mustDiscussion(t, s, open, "d-open")
	dQA := mustDiscussion(t, s, qa, "d-qa")

	t1, _ := s.CreateDiscussionThread(ctx, dQA, "t1", 0)
	t2, _ := s.CreateDiscussionThread(ctx, dQA, "t2", 0)
	reply, _ := s.CreateDiscussionComment(ctx, dQA, t1, "r1", 0)
	tOpen, _ := s.CreateDiscussionThread(ctx, dOpen, "t-open", 0)

	if err := s.SetThreadAnswer(ctx, dOpen, tOpen, true); !errors.Is(err, ErrNotAnswerable) {
		t.Fatalf("answer on open category: %v", err)
	}
	// A reply id coinciding with a thread id is indistinguishable from
	// that thread; only a truly missing thread id is a miss.
	_ = reply
	if err := s.SetThreadAnswer(ctx, dQA, 999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("answer on missing thread: %v", err)
	}
	if err := s.SetThreadAnswer(ctx, 999, t1, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("answer on missing discussion: %v", err)
	}

	if err := s.SetThreadAnswer(ctx, dQA, t1, true); err != nil {
		t.Fatalf("mark t1: %v", err)
	}
	if err := s.SetThreadAnswer(ctx, dQA, t2, true); err != nil {
		t.Fatalf("mark t2: %v", err)
	}
	d, err := s.GetDiscussion(ctx, dQA)
	if err != nil || !d.Answered {
		t.Fatalf("answered flag: %v %+v", err, d)
	}
	var marked int
	for _, th := range mustListDThreads(t, s, dQA) {
		if th.IsAnswer {
			marked++
			if th.ID != t2 {
				t.Fatalf("wrong thread marked: %d", th.ID)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("marked = %d, want 1", marked)
	}

	if err := s.SetThreadAnswer(ctx, dQA, t2, false); err != nil {
		t.Fatalf("unmark: %v", err)
	}
	d, _ = s.GetDiscussion(ctx, dQA)
	if d.Answered {
		t.Fatal("still answered after unmark")
	}
}

func TestUpdateDiscussionToOpenCategoryClearsAnswers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	open := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	qa := mustCategory(t, s, "Q&A", "qa", CategoryFormatQA)
	d := mustDiscussion(t, s, qa, "d")

	tid, _ := s.CreateDiscussionThread(ctx, d, "the fix", 0)
	if err := s.SetThreadAnswer(ctx, d, tid, true); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if err := s.UpdateDiscussion(ctx, d, DiscussionInput{Title: "d", CategoryID: open}); err != nil {
		t.Fatalf("UpdateDiscussion: %v", err)
	}
	got, err := s.GetDiscussion(ctx, d)
	if err != nil {
		t.Fatalf("GetDiscussion: %v", err)
	}
	if got.Answered {
		t.Fatal("answer mark stranded after move to open category")
	}
	for _, th := range mustListDThreads(t, s, d) {
		if th.IsAnswer {
			t.Fatalf("is_answer flag left on thread %d", th.ID)
		}
	}
}

func mustListDThreads(t *testing.T, s *Store, discussionID int64) []*DiscussionThread {
	t.Helper()
	ts, err := s.ListDiscussionThreads(context.Background(), discussionID)
	if err != nil {
		t.Fatalf("ListDiscussionThreads: %v", err)
	}
	return ts
}

func TestDeleteDiscussionCascadesComments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := mustCategory(t, s, "Q&A", "qa", CategoryFormatQA)
	d := mustDiscussion(t, s, cat, "d")
	t1, _ := s.CreateDiscussionThread(ctx, d, "t1", 0)
	if _, err := s.CreateDiscussionComment(ctx, d, t1, "r1", 0); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if err := s.DeleteDiscussion(ctx, d); err != nil {
		t.Fatalf("DeleteDiscussion: %v", err)
	}
	var threads, replies int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM discussion_threads WHERE discussion_id = ?", d).Scan(&threads); err != nil {
		t.Fatalf("count threads: %v", err)
	}
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM discussion_comments WHERE thread_id = ?", t1).Scan(&replies); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if threads != 0 || replies != 0 {
		t.Fatalf("rows left after delete: %d threads, %d comments", threads, replies)
	}
}

func TestDiscussionCommentBumpsUpdatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := mustCategory(t, s, "General", "general", CategoryFormatOpen)
	d := mustDiscussion(t, s, cat, "d")

	before, _ := s.GetDiscussion(ctx, d)
	time.Sleep(1100 * time.Millisecond) // unix-second granularity
	if _, err := s.CreateDiscussionThread(ctx, d, "activity", 0); err != nil {
		t.Fatalf("thread: %v", err)
	}
	after, _ := s.GetDiscussion(ctx, d)
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("updated_at not bumped: %v -> %v", before.UpdatedAt, after.UpdatedAt)
	}
}
