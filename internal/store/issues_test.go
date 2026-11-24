package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mustLabel inserts a workspace-1 issue label; master labels of the same
// name coexist per the union semantics.
func mustLabel(t *testing.T, s *Store, name, color string) int64 {
	t.Helper()
	res, err := s.db.Exec("INSERT INTO labels(workspace_id, kind, name, color) VALUES(1, 'issue', ?, ?)", name, color)
	if err != nil {
		t.Fatalf("insert label: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// backdate moves created_at/updated_at of an issue age into the past.
func backdate(t *testing.T, s *Store, id int64, age time.Duration) {
	t.Helper()
	ts := time.Now().UTC().Add(-age).Unix()
	if _, err := s.db.Exec("UPDATE issues SET created_at = ?, updated_at = ? WHERE id = ?", ts, ts, id); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

func labelNames(i *Issue) []string {
	var out []string
	for _, l := range i.Labels {
		out = append(out, l.Name)
	}
	return out
}

func issueIDs(issues []*Issue) []int64 {
	out := make([]int64, len(issues))
	for i, is := range issues {
		out[i] = is.ID
	}
	return out
}

func equalIDs(got []int64, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestCreateAndGetIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bug := mustLabel(t, s, "Bug", "#d73a4a")
	feat := mustLabel(t, s, "Feature", "#0075ca")

	id, err := s.CreateIssue(ctx, 1, IssueInput{
		Title: "first issue", Body: "hello body", LabelIDs: []int64{feat, bug},
	}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	i, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if i.Title != "first issue" || i.Body != "hello body" {
		t.Fatalf("unexpected issue: %+v", i)
	}
	if i.Status != StatusOpen {
		t.Fatalf("status = %q, want open", i.Status)
	}
	if i.ID != id {
		t.Fatalf("id = %d, want %d", i.ID, id)
	}
	names := labelNames(i)
	if len(names) != 2 || names[0] != "Bug" || names[1] != "Feature" {
		t.Fatalf("labels = %v, want [Bug Feature] (sorted by name)", names)
	}
	if i.Labels[0].Color != "#d73a4a" {
		t.Fatalf("label color = %q", i.Labels[0].Color)
	}
	if time.Since(i.CreatedAt) > time.Minute || i.CreatedAt.Location() != time.UTC {
		t.Fatalf("created_at = %v, want recent UTC", i.CreatedAt)
	}
}

func TestIssueIDsNotReused(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id1, err := s.CreateIssue(ctx, 1, IssueInput{Title: "one"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := s.DeleteIssue(ctx, id1); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	id2, err := s.CreateIssue(ctx, 1, IssueInput{Title: "two"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if id2 <= id1 {
		t.Fatalf("id reused: %d after %d", id2, id1)
	}
}

func TestUpdateIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bug := mustLabel(t, s, "Bug", "#d73a4a")
	feat := mustLabel(t, s, "Feature", "#0075ca")
	doc := mustLabel(t, s, "Docs", "#1f6feb")

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "old", Body: "old body", LabelIDs: []int64{bug, feat}}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	backdate(t, s, id, 24*time.Hour)

	err = s.UpdateIssue(ctx, id, IssueInput{Title: "new", Body: "new body", LabelIDs: []int64{doc}})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	i, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if i.Title != "new" || i.Body != "new body" {
		t.Fatalf("fields not updated: %+v", i)
	}
	if names := labelNames(i); len(names) != 1 || names[0] != "Docs" {
		t.Fatalf("labels = %v, want [Docs]", names)
	}
	if !i.UpdatedAt.After(i.CreatedAt) {
		t.Fatalf("updated_at %v not bumped past created_at %v", i.UpdatedAt, i.CreatedAt)
	}
}

// TestIssueLabelsSkipUnknownAndDuplicates verifies that unknown and
// duplicated label ids are silently dropped instead of failing with
// constraint errors.
func TestIssueLabelsSkipUnknownAndDuplicates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bug := mustLabel(t, s, "Bug", "#d73a4a")

	id, err := s.CreateIssue(ctx, 1, IssueInput{
		Title: "x", LabelIDs: []int64{bug, bug, 99999},
	}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	i, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if len(i.Labels) != 1 || i.Labels[0].ID != bug {
		t.Fatalf("labels = %+v, want [Bug]", i.Labels)
	}

	if err := s.UpdateIssue(ctx, id, IssueInput{Title: "x", LabelIDs: []int64{99999}}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	i, err = s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if len(i.Labels) != 0 {
		t.Fatalf("labels = %+v, want empty", i.Labels)
	}
}

func TestSetIssueStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "x"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	backdate(t, s, id, time.Hour)

	if err := s.SetIssueStatus(ctx, id, StatusClosed); err != nil {
		t.Fatalf("SetIssueStatus: %v", err)
	}
	i, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if i.Status != StatusClosed {
		t.Fatalf("status = %q, want closed", i.Status)
	}
	if !i.UpdatedAt.After(i.CreatedAt) {
		t.Fatalf("updated_at not bumped")
	}

	if err := s.SetIssueStatus(ctx, id, StatusOpen); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := s.SetIssueStatus(ctx, id, "bogus"); err == nil {
		t.Fatal("expected error for invalid status")
	} else if errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid status should not yield ErrNotFound: %v", err)
	}
}

// TestSetIssueStatusUpdatedAt verifies updated_at is bumped only by an
// actual status change, while ErrNotFound still reports missing issues.
func TestSetIssueStatusUpdatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "x"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	backdate(t, s, id, time.Hour)
	before, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	if err := s.SetIssueStatus(ctx, id, StatusOpen); err != nil {
		t.Fatalf("SetIssueStatus(same): %v", err)
	}
	same, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if !same.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("no-op status bumped updated_at: %v -> %v", before.UpdatedAt, same.UpdatedAt)
	}

	if err := s.SetIssueStatus(ctx, id, StatusClosed); err != nil {
		t.Fatalf("SetIssueStatus: %v", err)
	}
	changed, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if !changed.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("updated_at not bumped: %v -> %v", before.UpdatedAt, changed.UpdatedAt)
	}

	if err := s.SetIssueStatus(ctx, 4242, StatusClosed); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetIssueStatus missing = %v, want ErrNotFound", err)
	}
}

func TestDeleteIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bug := mustLabel(t, s, "Bug", "#d73a4a")
	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "gone", LabelIDs: []int64{bug}}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if _, err := s.CreateComment(ctx, id, "a comment", 0); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if err := s.DeleteIssue(ctx, id); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	if _, err := s.GetIssue(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetIssue after delete = %v, want ErrNotFound", err)
	}
	cs, err := s.ListComments(ctx, id)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(cs) != 0 {
		t.Fatalf("comments not cascaded: %v", cs)
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM issue_labels WHERE issue_id = ?", id).Scan(&n); err != nil {
		t.Fatalf("count issue_labels: %v", err)
	}
	if n != 0 {
		t.Fatalf("label links not cascaded: %d rows", n)
	}
	if err := s.DeleteIssue(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestComments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "threaded"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	for _, b := range []string{"first", "second", "third"} {
		if _, err := s.CreateComment(ctx, id, b, 0); err != nil {
			t.Fatalf("CreateComment(%q): %v", b, err)
		}
	}
	// Make timestamps distinct so ordering by created_at is exercised.
	old := time.Now().UTC().Add(-time.Hour).Unix()
	if _, err := s.db.Exec("UPDATE comments SET created_at = ? WHERE body = 'third'", old); err != nil {
		t.Fatalf("backdate comment: %v", err)
	}

	cs, err := s.ListComments(ctx, id)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("got %d comments, want 3", len(cs))
	}
	if cs[0].Body != "third" || cs[1].Body != "first" || cs[2].Body != "second" {
		t.Fatalf("order = %q,%q,%q", cs[0].Body, cs[1].Body, cs[2].Body)
	}
	for _, c := range cs {
		if c.IssueID != id || c.ID <= 0 {
			t.Fatalf("bad comment: %+v", c)
		}
	}
}

// TestCreateCommentBumpsIssueUpdatedAt verifies that a new comment counts
// as activity on its issue.
func TestCreateCommentBumpsIssueUpdatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "x"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	backdate(t, s, id, time.Hour)
	before, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	cid, err := s.CreateComment(ctx, id, "ping", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	after, err := s.GetIssue(ctx, id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("comment did not bump updated_at: %v -> %v", before.UpdatedAt, after.UpdatedAt)
	}
	cs, err := s.ListComments(ctx, id)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(cs) != 1 || cs[0].ID != cid {
		t.Fatalf("comments = %+v, want 1 comment id=%d", cs, cid)
	}
	if after.UpdatedAt.Before(cs[0].CreatedAt) {
		t.Fatalf("updated_at %v before comment created_at %v", after.UpdatedAt, cs[0].CreatedAt)
	}
}

func TestCreateCommentMissingIssue(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateComment(context.Background(), 9999, "hi", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateComment = %v, want ErrNotFound", err)
	}
}

func TestNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.GetIssue(ctx, 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetIssue = %v", err)
	}
	if err := s.UpdateIssue(ctx, 42, IssueInput{Title: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateIssue = %v", err)
	}
	if err := s.SetIssueStatus(ctx, 42, StatusClosed); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetIssueStatus = %v", err)
	}
	if err := s.DeleteIssue(ctx, 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteIssue = %v", err)
	}
}

func TestListIssues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bug := mustLabel(t, s, "Bug", "#d73a4a")
	feat := mustLabel(t, s, "Feature", "#0075ca")

	mk := func(title, body string, age time.Duration, labels ...int64) int64 {
		id, err := s.CreateIssue(ctx, 1, IssueInput{Title: title, Body: body, LabelIDs: labels}, 0)
		if err != nil {
			t.Fatalf("CreateIssue(%q): %v", title, err)
		}
		backdate(t, s, id, age)
		return id
	}

	i1 := mk("Fix crash on startup", "boom", 3*24*time.Hour, bug)
	i2 := mk("Add export feature", "needs csv dump support", 2*24*time.Hour, feat)
	i3 := mk("Improve docs", "more examples", 24*time.Hour)
	i4 := mk("Crash report details", "contains a stacktrace", 4*24*time.Hour, bug)
	if err := s.SetIssueStatus(ctx, i3, StatusClosed); err != nil {
		t.Fatalf("SetIssueStatus: %v", err)
	}

	check := func(name string, f ListFilter, wantTotal int, wantIDs ...int64) {
		t.Helper()
		issues, total, err := s.ListIssues(ctx, 1, f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if total != wantTotal {
			t.Fatalf("%s: total = %d, want %d", name, total, wantTotal)
		}
		if !equalIDs(issueIDs(issues), wantIDs...) {
			t.Fatalf("%s: ids = %v, want %v", name, issueIDs(issues), wantIDs)
		}
	}

	check("default status is open", ListFilter{}, 3, i2, i1, i4)
	check("status all", ListFilter{Status: "all"}, 4, i3, i2, i1, i4)
	check("status closed", ListFilter{Status: StatusClosed}, 1, i3)
	check("query hits title (case-insensitive)", ListFilter{Query: "CRASH"}, 2, i1, i4)
	check("query hits body only", ListFilter{Query: "stacktrace"}, 1, i4)
	check("query hits body of another issue", ListFilter{Query: "csv"}, 1, i2)
	check("query no match", ListFilter{Query: "zzz"}, 0)
	check("label filter bug", ListFilter{LabelID: bug}, 2, i1, i4)
	check("label filter feature", ListFilter{LabelID: feat}, 1, i2)
	check("oldest first", ListFilter{Status: "all", Sort: "oldest"}, 4, i4, i1, i2, i3)
	check("newest explicit", ListFilter{Status: "all", Sort: "newest"}, 4, i3, i2, i1, i4)

	issues, total, err := s.ListIssues(ctx, 1, ListFilter{Status: "all", PerPage: 2, Page: 1})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if total != 4 || !equalIDs(issueIDs(issues), i3, i2) {
		t.Fatalf("page1: ids=%v total=%d", issueIDs(issues), total)
	}
	issues, _, err = s.ListIssues(ctx, 1, ListFilter{Status: "all", PerPage: 2, Page: 2})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if !equalIDs(issueIDs(issues), i1, i4) {
		t.Fatalf("page2: ids=%v", issueIDs(issues))
	}
	issues, total, err = s.ListIssues(ctx, 1, ListFilter{Status: "all", PerPage: 2, Page: 3})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if total != 4 || len(issues) != 0 {
		t.Fatalf("page3: len=%d total=%d", len(issues), total)
	}
	issues, _, err = s.ListIssues(ctx, 1, ListFilter{Status: "all", PerPage: 2, Page: 0})
	if err != nil {
		t.Fatalf("page0: %v", err)
	}
	if !equalIDs(issueIDs(issues), i3, i2) {
		t.Fatalf("page0: ids=%v", issueIDs(issues))
	}

	issues, _, err = s.ListIssues(ctx, 1, ListFilter{LabelID: bug})
	if err != nil {
		t.Fatalf("label populated: %v", err)
	}
	if len(issues[0].Labels) != 1 || issues[0].Labels[0].Name != "Bug" {
		t.Fatalf("labels not populated: %+v", issues[0].Labels)
	}
}

func TestListLabels(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mustLabel(t, s, "Zeta", "#000000")
	mustLabel(t, s, "Alpha", "#ffffff")
	mustLabel(t, s, "Mid", "#888888")

	// ws 1 sees the four schema-seeded master labels unioned with its own.
	ls, err := s.ListLabels(ctx, 1, KindLabelIssue)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	want := []string{"Alpha", "Bug", "Documentation", "Enhancement", "Feature", "Mid", "Zeta"}
	if len(ls) != len(want) {
		t.Fatalf("ListLabels = %d rows, want %d: %+v", len(ls), len(want), ls)
	}
	for i, l := range ls {
		if l.Name != want[i] {
			t.Fatalf("not sorted by name: %+v", ls)
		}
	}

	// Another workspace sees only master labels — ws-1 rows stay scoped.
	ls2, err := s.ListLabels(ctx, 2, KindLabelIssue)
	if err != nil {
		t.Fatalf("ListLabels ws2: %v", err)
	}
	if len(ls2) != 4 {
		t.Fatalf("ListLabels ws2 = %d rows, want 4 master labels: %+v", len(ls2), ls2)
	}
}
