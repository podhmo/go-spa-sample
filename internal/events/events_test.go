package events

import (
	"testing"
	"time"
)

// recv waits for one change; a missed delivery fails the test rather than
// hanging it.
func recv(t *testing.T, ch <-chan Change) Change {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for change")
		return Change{}
	}
}

func TestHubPublishToSubscribedTopic(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(IssueTopic(3))
	defer unsub()

	h.Publish(Change{Topic: IssueTopic(3), Kind: KindComment, Target: 11})
	c := recv(t, ch)
	if c.Kind != KindComment || c.Target != 11 || c.Seq != 1 {
		t.Fatalf("got %+v", c)
	}
}

func TestHubIgnoresOtherTopics(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(IssueTopic(3))
	defer unsub()

	h.Publish(Change{Topic: IssueTopic(4), Kind: KindComment, Target: 11})
	h.Publish(Change{Topic: DiscussionTopic(3), Kind: KindThread, Target: 5})
	select {
	case c := <-ch:
		t.Fatalf("unrelated delivery: %+v", c)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHubSeqIncreases(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(IssuesTopic)
	defer unsub()

	h.Publish(Change{Topic: IssuesTopic, Kind: KindNew, Target: 1})
	h.Publish(Change{Topic: IssuesTopic, Kind: KindNew, Target: 2})
	if a, b := recv(t, ch), recv(t, ch); b.Seq != a.Seq+1 {
		t.Fatalf("seq did not increase: %d then %d", a.Seq, b.Seq)
	}
}

// A subscriber that never reads collapses its backlog to a resync marker
// instead of blocking the publisher — the whole point of the buffer cap.
func TestHubOverflowCollapsesToResync(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(IssueTopic(1))
	defer unsub()

	for i := range subBuffer + 5 {
		h.Publish(Change{Topic: IssueTopic(1), Kind: KindComment, Target: int64(i)})
	}
	got := recv(t, ch)
	if got.Kind != KindResync {
		t.Fatalf("overflow did not collapse to resync: %+v", got)
	}
}

func TestHubUnsubscribe(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(IssuesTopic)
	unsub()
	unsub() // second call must be a no-op

	h.Publish(Change{Topic: IssuesTopic, Kind: KindNew, Target: 1})
	select {
	case c := <-ch:
		t.Fatalf("delivery after unsubscribe: %+v", c)
	case <-time.After(50 * time.Millisecond):
	}
}
