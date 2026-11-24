// Package events carries the live-update notification contract shared by
// the store (publisher) and the web layer (SSE broadcaster):
// docs/sketch/ja/live-updates.md. Changes are volatile hints — "something
// changed, re-fetch" — never the content itself; the store stays the only
// source of truth and missed hints are recovered by a client resync.
package events

import (
	"strconv"
	"sync"
)

// Change kinds delivered to SSE subscribers.
const (
	KindComment = "comment" // issue comment added/edited/hidden (Target = comment id)
	KindThread  = "thread"  // discussion thread added/edited/hidden (Target = thread id)
	KindReply   = "reply"   // discussion reply added/edited/hidden (Parent = thread id)
	KindAnswer  = "answer"  // answer mark moved (Target = marked, Parent = unmarked)
	KindStatus  = "status"  // issue open/closed toggled
	KindEdit    = "edit"    // issue/discussion title, body or labels changed
	KindNew     = "new"     // new issue/discussion (list topics only)
	KindDelete  = "delete"  // the watched issue/discussion was deleted
	// KindRemove covers comment-level rows deleted below a watched
	// resource (issue comment, thread, reply — Target = row id, Parent =
	// thread id for replies, 0 otherwise); subscribers drop that node.
	KindRemove = "remove"
	// KindResync is synthesized by the Hub itself: a subscriber that fell
	// behind gets its backlog replaced by this single hint meaning "fetch
	// the whole list again". Stores never publish it.
	KindResync = "resync"
)

// Topic names a channel of related changes. Detail pages subscribe to the
// per-resource topic; list pages (future) subscribe to the list topics.
const (
	IssuesTopic      = "issues"
	DiscussionsTopic = "discussions"
	PullsTopic       = "pulls"
	// AllTopic is the fan-in channel carrying every change regardless of
	// resource. The SPA server streams it on /api/v1/events: the client
	// invalidates its loaders on any hint, so per-resource topics are
	// not needed there (main.go re-publishes each change under both).
	AllTopic = "all"
)

// IssueTopic is the per-issue channel ("issue:5").
func IssueTopic(id int64) string { return "issue:" + strconv.FormatInt(id, 10) }

// DiscussionTopic is the per-discussion channel ("discussion:3").
func DiscussionTopic(id int64) string { return "discussion:" + strconv.FormatInt(id, 10) }

// PullRequestTopic is the per-pull-request channel ("pr:5").
func PullRequestTopic(id int64) string { return "pr:" + strconv.FormatInt(id, 10) }

// Change is one mutation hint. Seq is assigned by Hub.Publish and is
// monotonically increasing per process, so subscribers can dedupe and
// (on reconnect) compare last-seen positions.
type Change struct {
	Seq    int64  `json:"seq"`
	Topic  string `json:"-"`
	Kind   string `json:"kind"`
	Target int64  `json:"target"`
	Parent int64  `json:"parent,omitempty"`
}

// subBuffer caps a subscriber's backlog before the hub collapses it to a
// resync marker.
const subBuffer = 32

// Hub is an in-process fan-out: Publish writes to every channel
// registered under the change's Topic. It is a synchronous, non-blocking
// broadcast — a slow subscriber never stalls a writer; its backlog is
// instead collapsed into a single resync marker so writers (HTTP POST
// handlers) never queue behind a stuck browser.
type Hub struct {
	mu   sync.Mutex
	seq  int64
	subs map[string]map[chan Change]struct{}
}

// NewHub returns a Hub with no subscribers.
func NewHub() *Hub {
	return &Hub{subs: map[string]map[chan Change]struct{}{}}
}

// Publish assigns the change a sequence number and delivers it to every
// current subscriber of its topic. Safe for concurrent use; intended to
// be wired as (*store.Store).WatchChanges(ctx, watermark, hub.Publish).
func (h *Hub) Publish(c Change) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	c.Seq = h.seq
	for ch := range h.subs[c.Topic] {
		select {
		case ch <- c:
		default:
			// The subscriber fell behind: drain its backlog and leave a
			// single resync marker — losing individual events is fine
			// because resync refetches the whole list anyway.
			for {
				select {
				case <-ch:
				default:
					ch <- Change{Seq: h.seq, Topic: c.Topic, Kind: KindResync}
					goto next
				}
			}
		next:
		}
	}
}

// Subscribe registers a buffered channel for the topic and returns it
// plus an unsubscribe func. Every subscriber must eventually call the
// returned func or the channel leaks.
func (h *Hub) Subscribe(topic string) (<-chan Change, func()) {
	ch := make(chan Change, subBuffer)
	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = map[chan Change]struct{}{}
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs[topic], ch)
			if len(h.subs[topic]) == 0 {
				delete(h.subs, topic)
			}
			h.mu.Unlock()
		})
	}
}
