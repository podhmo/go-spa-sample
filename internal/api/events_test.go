package api

import (
	"path/filepath"

	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// The events stream emits every hub-published AllTopic change as an SSE
// frame and stays open for more; a write to the store from any process
// shows up once WatchChanges feeds the hub.
func TestEventsStream(t *testing.T) {
	hub := events.NewHub()
	srv := newServerWithHub(t, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type: got %q", ct)
	}

	// The retry directive arrives first, then frames for published hints.
	lines := bufio.NewScanner(resp.Body)
	readFrame := func(timeout time.Duration) string {
		t.Helper()
		type result struct{ line string }
		ch := make(chan result, 1)
		go func() {
			for lines.Scan() {
				if line := lines.Text(); line != "" {
					ch <- result{line}
					return
				}
			}
			ch <- result{}
		}()
		select {
		case r := <-ch:
			return r.line
		case <-time.After(timeout):
			t.Fatal("timed out waiting for SSE frame")
			return ""
		}
	}

	if line := readFrame(2 * time.Second); line != "retry: 3000" {
		t.Fatalf("first frame: got %q", line)
	}

	hub.Publish(events.Change{Topic: events.AllTopic, Kind: events.KindComment, Target: 42})
	got := readFrame(2 * time.Second)
	if !strings.HasPrefix(got, "id: ") {
		t.Fatalf("id line: got %q", got)
	}
	data := readFrame(2 * time.Second)
	if !strings.Contains(data, `"kind":"comment"`) || !strings.Contains(data, `"target":42`) {
		t.Fatalf("data line: got %q", data)
	}
}

// Without a hub the endpoint is unmounted.
func TestEventsStreamNoHub(t *testing.T) {
	srv := newServerWithHub(t, nil)

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", resp.StatusCode)
	}
}

// newServerWithHub boots the API handler with the given hub (nil allowed)
// over a fresh store.
func newServerWithHub(t *testing.T, hub *events.Hub) *httptest.Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(New(st, testToken, nil, hub))
	t.Cleanup(srv.Close)
	return srv
}

// The delivery path the browser relies on end to end: a write through
// the store (the shape the local CLI uses from its own process) lands in
// the changes table, WatchChanges re-publishes it, and the SSE stream
// emits the frame.
func TestEventsStreamFromStoreWrite(t *testing.T) {
	hub := events.NewHub()
	srv, st := newServerWithHubStore(t, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watermark, err := st.LatestChangeSeq(ctx)
	if err != nil {
		t.Fatal(err)
	}
	go st.WatchChanges(ctx, watermark, func(c events.Change) {
		hub.Publish(c)
		all := c
		all.Topic = events.AllTopic
		hub.Publish(all)
	})

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	ws, err := st.WorkspaceBySlug(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	issueID, err := st.CreateIssue(ctx, ws.ID, store.IssueInput{Title: "t", Body: "b"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateComment(ctx, issueID, "written as the CLI would", 0); err != nil {
		t.Fatal(err)
	}

	// Scan in a goroutine: the deadline cannot fire while Scan blocks,
	// and a missing frame would otherwise hang the test until the
	// heartbeat interval (or forever).
	lines := bufio.NewScanner(resp.Body)
	frames := make(chan string, 8)
	go func() {
		for lines.Scan() {
			frames <- lines.Text()
		}
		close(frames)
	}()
	deadline := time.After(3 * time.Second)
	var data string
loop:
	for {
		select {
		case line, ok := <-frames:
			if !ok {
				break loop
			}
			if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"kind":"comment"`) {
				data = line
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	if data == "" {
		t.Fatal("no comment frame streamed after the store write")
	}
}

// newServerWithHubStore is newServerWithHub but also returns the store,
// for tests that write through it directly.
func newServerWithHubStore(t *testing.T, hub *events.Hub) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(New(st, testToken, nil, hub))
	t.Cleanup(srv.Close)
	return srv, st
}
