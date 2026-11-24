// Command fakeapi is a throwaway API server for the SPA connectivity
// spike. Its endpoints are intentionally unrelated to zatudan.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	flag.Parse()

	var count atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "pong",
			"time":    time.Now().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("GET /api/counter", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"count": count.Load()})
	})
	mux.HandleFunc("POST /api/counter/increment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"count": count.Add(1)})
	})

	log.Printf("fakeapi listening on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
