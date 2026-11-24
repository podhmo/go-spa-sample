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
