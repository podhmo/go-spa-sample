package client

import (
	"context"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// The remote-mode path for the discussion subcommands: every cli.Backend
// method the discussion commands use, end to end over httptest.
func TestClientDiscussionLifecycle(t *testing.T) {
	c, st := newFixture(t, "env-tok")
	if err := st.SeedDemoData(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	cats, err := c.ListDiscussionCategories(ctx)
	if err != nil {
		t.Fatalf("ListDiscussionCategories: %v", err)
	}
	var qaID int64
	for _, cat := range cats {
		if cat.Slug == "qa" {
			qaID = cat.ID
		}
	}
	if qaID == 0 {
		t.Fatalf("qa category missing: %+v", cats)
	}

	id, err := c.CreateDiscussion(ctx, store.DiscussionInput{
		Title: "remote qa", Body: "body", CategoryID: qaID,
	}, 0)
	if err != nil {
		t.Fatalf("CreateDiscussion: %v", err)
	}

	d, err := c.GetDiscussion(ctx, id)
	if err != nil {
		t.Fatalf("GetDiscussion: %v", err)
	}
	if d.Title != "remote qa" || d.Category.Slug != "qa" || d.Answered {
		t.Fatalf("GetDiscussion = %+v", d)
	}

	list, total, err := c.ListDiscussions(ctx, store.DiscussionListFilter{
		CategorySlug: "qa", Query: "remote qa",
	})
	if err != nil {
		t.Fatalf("ListDiscussions: %v", err)
	}
	if len(list) != 1 || total != 1 || list[0].ID != id {
		t.Fatalf("ListDiscussions = %+v total %d", list, total)
	}
	list, _, err = c.ListDiscussions(ctx, store.DiscussionListFilter{Answered: "yes"})
	if err != nil {
		t.Fatalf("ListDiscussions answered=yes: %v", err)
	}
	for _, got := range list {
		if got.ID == id {
			t.Fatalf("unanswered discussion %d in answered=yes list", id)
		}
	}

	tid, err := c.CreateDiscussionThread(ctx, id, "the answer", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionThread: %v", err)
	}
	rid, err := c.CreateDiscussionComment(ctx, id, tid, "a reply", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionComment reply: %v", err)
	}
	ts, err := c.ListDiscussionThreads(ctx, id)
	if err != nil {
		t.Fatalf("ListDiscussionThreads: %v", err)
	}
	if len(ts) != 1 || ts[0].ID != tid || len(ts[0].Comments) != 1 || ts[0].Comments[0].ID != rid {
		t.Fatalf("ListDiscussionThreads = %+v", ts)
	}
	if ts[0].Comments[0].ThreadID != tid {
		t.Fatalf("reply thread = %d, want %d", ts[0].Comments[0].ThreadID, tid)
	}

	if err := c.SetThreadAnswer(ctx, id, tid, true); err != nil {
		t.Fatalf("SetThreadAnswer: %v", err)
	}
	d, _ = c.GetDiscussion(ctx, id)
	if !d.Answered {
		t.Fatal("discussion not answered after SetThreadAnswer")
	}
	if err := c.SetThreadAnswer(ctx, id, tid, false); err != nil {
		t.Fatalf("SetThreadAnswer unmark: %v", err)
	}

	if err := c.UpdateDiscussion(ctx, id, store.DiscussionInput{
		Title: "renamed qa", Body: "b2", CategoryID: qaID,
	}); err != nil {
		t.Fatalf("UpdateDiscussion: %v", err)
	}
	d, _ = c.GetDiscussion(ctx, id)
	if d.Title != "renamed qa" {
		t.Fatalf("title = %q", d.Title)
	}

	if err := c.DeleteDiscussion(ctx, id); err != nil {
		t.Fatalf("DeleteDiscussion: %v", err)
	}
	if _, err := c.GetDiscussion(ctx, id); err == nil {
		t.Fatal("GetDiscussion after delete: want error")
	}
}
