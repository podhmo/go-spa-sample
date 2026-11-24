package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

// The changes table is the live-updates channel: every committed mutation
// must land there with the right topic/kind/target so open pages — served
// by any process watching the file — can refetch.
func TestCommitTxRecordsChange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "n"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	cid, err := s.CreateComment(ctx, id, "hello", 0)
	if err != nil {
		t.Fatal(err)
	}

	changes, err := s.ListChanges(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("want 2 changes, got %d: %+v", len(changes), changes)
	}
	if c := changes[0]; c.Topic != events.IssuesTopic || c.Kind != events.KindNew || c.Target != id {
		t.Fatalf("got %+v", c)
	}
	if c := changes[1]; c.Topic != events.IssueTopic(id) || c.Kind != events.KindComment || c.Target != cid {
		t.Fatalf("got %+v", c)
	}
}

// Seq is the cross-process ordering of commits: ListChanges returns rows
// strictly after the given position, in order.
func TestListChangesOrdersAfterSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIssue(ctx, 1, IssueInput{Title: "n"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetIssueStatus(ctx, id, StatusClosed); err != nil {
		t.Fatal(err)
	}
	last, err := s.LatestChangeSeq(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateIssue(ctx, id, IssueInput{Title: "m"}); err != nil {
		t.Fatal(err)
	}

	changes, err := s.ListChanges(ctx, last)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != events.KindEdit {
		t.Fatalf("want just the edit, got %+v", changes)
	}
	if changes[0].Seq <= last {
		t.Fatalf("seq %d not after %d", changes[0].Seq, last)
	}
}

// SetThreadAnswer reports the previously marked thread in Parent so the
// client can re-render both <li>s.
func TestAnswerChangeReportsPreviousMark(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	qa := mustCategory(t, s, "Q&A", "qa", CategoryFormatQA)
	did, err := s.CreateDiscussion(ctx, 1, DiscussionInput{Title: "d", CategoryID: qa}, 0)
	if err != nil {
		t.Fatal(err)
	}
	t1, err := s.CreateDiscussionThread(ctx, did, "a", 0)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := s.CreateDiscussionThread(ctx, did, "b", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetThreadAnswer(ctx, did, t1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetThreadAnswer(ctx, did, t2, true); err != nil {
		t.Fatal(err)
	}

	changes, err := s.ListChanges(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := changes[len(changes)-1]
	if last.Topic != events.DiscussionTopic(did) || last.Kind != events.KindAnswer ||
		last.Target != t2 || last.Parent != t1 {
		t.Fatalf("got %+v, want answer target=%d parent=%d", last, t2, t1)
	}
}

// A rolled-back mutation must leave no hint behind — hints and mutations
// commit or vanish together.
func TestFailedMutationRecordsNoChange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateComment(ctx, 999, "nope", 0); err == nil {
		t.Fatal("want ErrNotFound")
	}
	changes, err := s.ListChanges(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("want no changes, got %+v", changes)
	}
}

// A database stamped user_version=1 before the changes table existed
// never re-runs migrateV1, so migrateV2 must add it — otherwise every
// commitTx insert fails and all writes roll back.
func TestMigrateV1DatabaseGainsChangesTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zatudan.db")
	s := mustOpen(t, path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Rewind the file to its pre-changes state: no table, version 1.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE changes"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s = mustOpen(t, path)
	if _, err := s.CreateIssue(context.Background(), 1, IssueInput{Title: "n"}, 0); err != nil {
		t.Fatalf("write on migrated v1 database: %v", err)
	}
}

// Two Store handles on the same file are the CLI↔server split in
// miniature: a write through one handle must surface through the other's
// watcher. This is the path that makes `issues --db` live-update pages.
func TestWatchChangesAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zatudan.db")
	server := mustOpen(t, path)
	cli := mustOpen(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	after, err := server.LatestChangeSeq(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan events.Change, 8)
	go server.watchChanges(ctx, 10*time.Millisecond, after, func(c events.Change) { got <- c })

	id, err := cli.CreateIssue(ctx, 1, IssueInput{Title: "from cli"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	cid, err := cli.CreateComment(ctx, id, "hello from cli", 0)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	var kinds []string
	for len(kinds) < 2 {
		select {
		case c := <-got:
			kinds = append(kinds, c.Kind)
			if c.Kind == events.KindComment && c.Target != cid {
				t.Fatalf("got %+v", c)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for cli changes; got %v", kinds)
		}
	}
}

// A watcher that starts after writes happened begins at the high-water
// mark — backlog is skipped, not replayed.
func TestWatchChangesSkipsBacklog(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := s.CreateIssue(ctx, 1, IssueInput{Title: "old"}, 0); err != nil {
		t.Fatal(err)
	}

	after, err := s.LatestChangeSeq(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan events.Change, 8)
	go s.watchChanges(ctx, 10*time.Millisecond, after, func(c events.Change) { got <- c })

	select {
	case c := <-got:
		t.Fatalf("backlog was replayed: %+v", c)
	case <-time.After(100 * time.Millisecond):
	}
}

func mustOpen(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open %q: %v", path, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
