package api

// Live updates: one SSE endpoint streaming every recorded change.
// Events are invalidation-only hints — {seq, kind, target, parent} — so
// the client just re-fetches its loaders (router.invalidate()); the
// store stays the only source of truth. The hub is fed by
// (*store.Store).WatchChanges, which tails the changes table and so
// covers every write path in one place: API mutations and, above all,
// the local CLI writing the SQLite file directly from its own process.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/podhmo/go-spa-sample/internal/events"
)

// sseHeartbeat keeps connections alive through idle proxies and lets the
// server notice dead clients.
const sseHeartbeat = 25 * time.Second

// streamEvents writes the SSE stream until the client disconnects.
// main.go sets WriteTimeout: 15s for the whole server — adequate for
// normal responses but fatal to a long-lived stream — so the per-response
// write deadline is lifted here (a zero deadline means none).
func (h *handler) streamEvents(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil {
		writeError(w, http.StatusNotFound, "Not found.", nil)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("api: sse events: cannot lift write deadline: %v", err)
	}

	// One subscriber channel fed with every topic: the hub keys channels
	// per topic, so a wildcard subscribe is not a hub feature — instead
	// this endpoint subscribes to the fan-in channel the watcher also
	// publishes to (main.go wires WatchChanges into both).
	ch, unsub := h.hub.Subscribe(events.AllTopic)
	defer unsub()

	// Send the retry directive immediately so the client's onopen fires
	// and its reconnect delay is ours, not the browser default.
	if _, err := fmt.Fprint(w, "retry: 3000\n\n"); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return
	}
	ping := time.NewTicker(sseHeartbeat)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case c := <-ch:
			data, err := json.Marshal(c)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", c.Seq, data); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
