package api

import (
	"fmt"
	"testing"
)

// Label lifecycle over the wire: create → DELETE (archive) → the
// archived listing param shows it while the active listing doesn't,
// and re-POSTing the same name/scope restores the archived row.
func TestLabelsArchivedListing(t *testing.T) {
	srv, _ := newServer(t)
	defer srv.Close()

	code, out := do(t, srv, "POST", "/api/v1/w/default/labels", `{"name":"temp","color":"#123456","kind":"issue"}`)
	if code != 201 {
		t.Fatalf("create = %d, out = %v", code, out)
	}
	id := int64(out["label"].(map[string]any)["id"].(float64))

	code, _ = do(t, srv, "DELETE", fmt.Sprintf("/api/v1/w/default/labels/%d", id), "")
	if code != 204 {
		t.Fatalf("archive = %d", code)
	}

	_, out = do(t, srv, "GET", "/api/v1/w/default/labels?archived=1", "")
	found := false
	for _, l := range out["labels"].([]any) {
		if l.(map[string]any)["name"] == "temp" {
			found = true
		}
	}
	if !found {
		t.Fatal("archived label missing from ?archived=1 listing")
	}

	_, out = do(t, srv, "GET", "/api/v1/w/default/labels?kind=issue", "")
	for _, l := range out["labels"].([]any) {
		if l.(map[string]any)["name"] == "temp" {
			t.Fatal("archived label leaked into the active listing")
		}
	}

	// Re-creating the same name in the same scope restores the row
	// (with the new color) instead of duplicating it.
	code, out = do(t, srv, "POST", "/api/v1/w/default/labels", `{"name":"temp","color":"#654321","kind":"issue"}`)
	if code != 201 {
		t.Fatalf("restore = %d, out = %v", code, out)
	}
	restored := out["label"].(map[string]any)
	if restored["id"].(float64) != float64(id) || restored["color"] != "#654321" || restored["archived"] != false {
		t.Fatalf("restored label = %v, want id %d color #654321", restored, id)
	}
}
