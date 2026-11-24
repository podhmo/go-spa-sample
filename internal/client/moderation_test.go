package client

import (
	"context"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// Round-trip the nine moderation methods over the real API: issue
// comments plus discussion threads/replies.
func TestClientCommentModeration(t *testing.T) {
	c, _ := newFixture(t, "env-tok")
	ctx := context.Background()

	id, err := c.CreateIssue(ctx, store.IssueInput{Title: "mod"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	cid, err := c.CreateComment(ctx, id, "first", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if err := c.UpdateComment(ctx, id, cid, "edited"); err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	cs, _ := c.ListComments(ctx, id)
	if cs[0].Body != "edited" || !cs[0].Moderation.Edited() {
		t.Fatalf("edited comment = %+v", cs[0].Moderation)
	}
	if err := c.SetCommentHidden(ctx, id, cid, "spam"); err != nil {
		t.Fatalf("hide: %v", err)
	}
	cs, _ = c.ListComments(ctx, id)
	if !cs[0].Hidden || cs[0].HiddenReason != "spam" {
		t.Fatalf("hidden comment = %+v", cs[0].Moderation)
	}
	// Empty reason selects the unhide endpoint.
	if err := c.SetCommentHidden(ctx, id, cid, ""); err != nil {
		t.Fatalf("unhide: %v", err)
	}
	cs, _ = c.ListComments(ctx, id)
	if cs[0].Hidden {
		t.Fatalf("unhidden comment = %+v", cs[0].Moderation)
	}
	if err := c.DeleteComment(ctx, id, cid); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if cs, _ := c.ListComments(ctx, id); len(cs) != 0 {
		t.Fatalf("comments after delete = %v", cs)
	}
}

// The discussion side: thread edit/hide/unhide/delete plus reply
// edit/hide/delete through the client.
func TestClientDiscussionModeration(t *testing.T) {
	c, st := newFixture(t, "env-tok")
	ctx := context.Background()
	if err := st.SeedDemoData(ctx); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	cats, err := st.ListDiscussionCategories(ctx)
	if err != nil || len(cats) == 0 {
		t.Fatalf("categories: %v", err)
	}

	did, err := c.CreateDiscussion(ctx, store.DiscussionInput{Title: "mod", CategoryID: cats[0].ID}, 0)
	if err != nil {
		t.Fatalf("CreateDiscussion: %v", err)
	}
	tid, err := c.CreateDiscussionThread(ctx, did, "opening", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionThread: %v", err)
	}
	rid, err := c.CreateDiscussionComment(ctx, did, tid, "a reply", 0)
	if err != nil {
		t.Fatalf("CreateDiscussionComment: %v", err)
	}

	if err := c.UpdateDiscussionThread(ctx, did, tid, "edited op"); err != nil {
		t.Fatalf("UpdateDiscussionThread: %v", err)
	}
	if err := c.SetDiscussionThreadHidden(ctx, did, tid, "duplicate"); err != nil {
		t.Fatalf("thread hide: %v", err)
	}
	ts, _ := c.ListDiscussionThreads(ctx, did)
	th := findThreadID(ts, tid)
	if th == nil || !th.Hidden || th.HiddenReason != "duplicate" || !th.Edited() {
		t.Fatalf("thread = %+v", th)
	}
	if err := c.SetDiscussionThreadHidden(ctx, did, tid, ""); err != nil {
		t.Fatalf("thread unhide: %v", err)
	}

	if err := c.UpdateDiscussionComment(ctx, did, tid, rid, "better"); err != nil {
		t.Fatalf("UpdateDiscussionComment: %v", err)
	}
	if err := c.SetDiscussionCommentHidden(ctx, did, tid, rid, "off-topic"); err != nil {
		t.Fatalf("reply hide: %v", err)
	}
	ts, _ = c.ListDiscussionThreads(ctx, did)
	th = findThreadID(ts, tid)
	if len(th.Comments) != 1 || !th.Comments[0].Hidden || th.Comments[0].HiddenReason != "off-topic" {
		t.Fatalf("reply = %+v", th.Comments[0])
	}
	if err := c.DeleteDiscussionComment(ctx, did, tid, rid); err != nil {
		t.Fatalf("DeleteDiscussionComment: %v", err)
	}
	if err := c.DeleteDiscussionThread(ctx, did, tid); err != nil {
		t.Fatalf("DeleteDiscussionThread: %v", err)
	}
	if ts, _ := c.ListDiscussionThreads(ctx, did); findThreadID(ts, tid) != nil {
		t.Fatal("thread should be gone")
	}
}

func findThreadID(ts []*store.DiscussionThread, id int64) *store.DiscussionThread {
	for _, t := range ts {
		if t.ID == id {
			return t
		}
	}
	return nil
}
