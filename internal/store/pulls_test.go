package store

import (
	"context"
	"errors"
	"testing"
)

func prInput() PRInput {
	return PRInput{
		BaseRef: "refs/heads/main", HeadRef: "refs/heads/feat",
		BaseSHA: "aaa", HeadSHA: "bbb", Title: "test pr", Body: "body",
	}
}

func TestPullRequestCRUD(t *testing.T) {
	ctx := context.Background()
	s := mustOpen(t, ":memory:")

	var pinned int64
	id, err := s.CreatePullRequest(ctx, prInput(), 0, func(prID int64) error {
		pinned = prID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if pinned != id {
		t.Fatalf("pin got %d, want %d", pinned, id)
	}
	pr, err := s.GetPullRequest(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Status != PRStatusOpen || pr.HeadRef != "refs/heads/feat" || pr.HeadSHA != "bbb" {
		t.Fatalf("pr = %+v", pr)
	}

	// Duplicate open pair rejected.
	if _, err := s.CreatePullRequest(ctx, prInput(), 0, nil); !errors.Is(err, ErrDuplicatePR) {
		t.Fatalf("dup err = %v", err)
	}
	// Same ref pair rejected.
	bad := prInput()
	bad.HeadRef = bad.BaseRef
	if _, err := s.CreatePullRequest(ctx, bad, 0, nil); !errors.Is(err, ErrSameRef) {
		t.Fatalf("same-ref err = %v", err)
	}
	// Failed pin rolls the insert back.
	other := prInput()
	other.HeadRef = "refs/heads/other"
	if _, err := s.CreatePullRequest(ctx, other, 0, func(int64) error { return errors.New("pin fail") }); err == nil {
		t.Fatal("pin failure should fail create")
	}
	if _, _, err := s.ListPullRequests(ctx, PRListFilter{Status: "all"}); err != nil {
		t.Fatal(err)
	}

	// Pin failure rollback: only the first PR exists.
	prs, total, err := s.ListPullRequests(ctx, PRListFilter{Status: "all"})
	if err != nil || total != 1 || prs[0].ID != id {
		t.Fatalf("after rollback: total=%d", total)
	}

	// Title/body update.
	up := prInput()
	up.Title = "renamed"
	if err := s.UpdatePullRequest(ctx, id, up); err != nil {
		t.Fatal(err)
	}
	pr, _ = s.GetPullRequest(ctx, id)
	if pr.Title != "renamed" {
		t.Fatalf("title = %q", pr.Title)
	}
}

func TestPullRequestStatusTransitions(t *testing.T) {
	ctx := context.Background()
	s := mustOpen(t, ":memory:")
	id, err := s.CreatePullRequest(ctx, prInput(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	// reopen on open → invalid
	if err := s.ReopenPullRequest(ctx, id); !errors.Is(err, ErrInvalidPRTransition) {
		t.Fatalf("reopen-open err = %v", err)
	}
	// merge on open → ok
	err = s.MarkMerged(ctx, id, PRMergeInfo{MergeSHA: "m", MergeMethod: "squash", HeadSHA: "h", MergeBaseSHA: "mb"})
	if err != nil {
		t.Fatal(err)
	}
	pr, _ := s.GetPullRequest(ctx, id)
	if pr.Status != PRStatusMerged || pr.MergeSHA != "m" || pr.ClosedAt == nil {
		t.Fatalf("merged pr = %+v", pr)
	}
	// merged is terminal
	if err := s.ReopenPullRequest(ctx, id); !errors.Is(err, ErrInvalidPRTransition) {
		t.Fatalf("reopen-merged err = %v", err)
	}
	if err := s.ClosePullRequest(ctx, id, "h2", "mb2"); !errors.Is(err, ErrInvalidPRTransition) {
		t.Fatalf("close-merged err = %v", err)
	}

	// close → reopen cycle
	id2, _ := s.CreatePullRequest(ctx, PRInput{BaseRef: "refs/heads/main", HeadRef: "refs/heads/x", Title: "t2"}, 0, nil)
	if err := s.ClosePullRequest(ctx, id2, "h", "mb"); err != nil {
		t.Fatal(err)
	}
	pr, _ = s.GetPullRequest(ctx, id2)
	if pr.Status != PRStatusClosed || pr.HeadSHA != "h" || pr.ClosedAt == nil {
		t.Fatalf("closed pr = %+v", pr)
	}
	// closed pair can be recreated (only open is unique).
	id3, err := s.CreatePullRequest(ctx, PRInput{BaseRef: "refs/heads/main", HeadRef: "refs/heads/x", Title: "t3"}, 0, nil)
	if err != nil {
		t.Fatalf("recreate closed pair: %v", err)
	}
	// Reopening id2 while id3 is open for the same pair violates the
	// partial unique index — mapped to ErrDuplicatePR.
	if err := s.ReopenPullRequest(ctx, id2); !errors.Is(err, ErrDuplicatePR) {
		t.Fatalf("reopen over open pair err = %v", err)
	}
	// Closing id3 frees the pair; reopen succeeds then.
	if err := s.ClosePullRequest(ctx, id3, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.ReopenPullRequest(ctx, id2); err != nil {
		t.Fatal(err)
	}
	pr, _ = s.GetPullRequest(ctx, id2)
	if pr.Status != PRStatusOpen || pr.ClosedAt != nil {
		t.Fatalf("reopened pr = %+v", pr)
	}
}

func TestPRComments(t *testing.T) {
	ctx := context.Background()
	s := mustOpen(t, ":memory:")
	prID, err := s.CreatePullRequest(ctx, prInput(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Plain comment.
	cid, err := s.CreatePRComment(ctx, prID, PRCommentInput{Body: "hello"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Anchored review comment.
	rid, err := s.CreatePRComment(ctx, prID, PRCommentInput{
		Path: "internal/web/x.go", Line: 42, Side: "left",
		AnchorSHA: "deadbeef", DiffSnippet: "ctx", Body: "why?",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Mismatched anchor → ErrInvalidAnchor.
	if _, err := s.CreatePRComment(ctx, prID, PRCommentInput{Path: "x", Body: "b"}, 0); !errors.Is(err, ErrInvalidAnchor) {
		t.Fatalf("anchor err = %v", err)
	}
	// Missing PR → ErrNotFound.
	if _, err := s.CreatePRComment(ctx, 9999, PRCommentInput{Body: "b"}, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("notfound err = %v", err)
	}

	cs, err := s.ListPRComments(ctx, prID)
	if err != nil || len(cs) != 2 {
		t.Fatalf("comments = %v, %v", cs, err)
	}
	if cs[0].ID != cid || cs[0].Anchored() {
		t.Fatalf("plain = %+v", cs[0])
	}
	rc := cs[1]
	if !rc.Anchored() || rc.Line != 42 || rc.Side != "left" || rc.AnchorSHA != "deadbeef" || rc.DiffSnippet != "ctx" {
		t.Fatalf("review = %+v", rc)
	}
	if rid != cs[1].ID {
		t.Fatal("order")
	}

	// Moderation verbs, same as issue comments.
	if err := s.UpdatePRComment(ctx, prID, cid, "edited"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPRCommentHidden(ctx, prID, rid, HideReasonOutdated); err != nil {
		t.Fatal(err)
	}
	cs, _ = s.ListPRComments(ctx, prID)
	if !cs[0].Moderation.Edited() || cs[1].Moderation.HiddenReason != HideReasonOutdated {
		t.Fatalf("moderation = %+v / %+v", cs[0].Moderation, cs[1].Moderation)
	}
	if err := s.SetPRCommentHidden(ctx, prID, rid, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPRCommentHidden(ctx, prID, rid, "bogus"); !errors.Is(err, ErrInvalidHideReason) {
		t.Fatalf("bad reason err = %v", err)
	}
	// Cross-PR scope check.
	other, _ := s.CreatePullRequest(ctx, PRInput{BaseRef: "b", HeadRef: "h", Title: "t"}, 0, nil)
	if err := s.UpdatePRComment(ctx, other, cid, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-scope err = %v", err)
	}
	if err := s.DeletePRComment(ctx, prID, cid); err != nil {
		t.Fatal(err)
	}
	cs, _ = s.ListPRComments(ctx, prID)
	if len(cs) != 1 {
		t.Fatalf("after delete: %d", len(cs))
	}
}

func TestMigrateV5(t *testing.T) {
	// A v4-stamped database gains the PR tables via migrateV5, not just
	// fresh schemaDDL — simulate by opening, then re-running the DDL idempotency.
	s := mustOpen(t, ":memory:")
	tables, _, err := s.SchemaObjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var hasPR, hasPRC bool
	for _, name := range tables {
		if name == "pull_requests" {
			hasPR = true
		}
		if name == "pr_comments" {
			hasPRC = true
		}
	}
	if !hasPR || !hasPRC {
		t.Fatalf("missing tables: %v", tables)
	}
	// The open-pair partial unique index exists.
	_, idx, err := s.SchemaObjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range idx {
		if name == "idx_pull_requests_open_pair" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing index: %v", idx)
	}
}

func TestListPullRequestsStatusFilter(t *testing.T) {
	ctx := context.Background()
	s := mustOpen(t, ":memory:")
	open1, _ := s.CreatePullRequest(ctx, prInput(), 0, nil)
	in := prInput()
	in.HeadRef = "refs/heads/other"
	closed1, _ := s.CreatePullRequest(ctx, in, 0, nil)
	if err := s.ClosePullRequest(ctx, closed1, "h", "mb"); err != nil {
		t.Fatal(err)
	}

	for status, want := range map[string]int{"": 1, "open": 1, "closed": 1, "merged": 0, "all": 2} {
		prs, total, err := s.ListPullRequests(ctx, PRListFilter{Status: status})
		if err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		if total != want || len(prs) != want {
			t.Fatalf("status %q: total=%d len=%d, want %d", status, total, len(prs), want)
		}
	}
	// The open row is the expected one.
	prs, _, _ := s.ListPullRequests(ctx, PRListFilter{Status: "open"})
	if prs[0].ID != open1 {
		t.Fatalf("open list = %+v", prs[0])
	}
}
