package api

import (
	"fmt"
	"net/http"
	"testing"
)

// Comment moderation on issue comments: PATCH rewrites the body and
// stamps edited_at, hide/unhide round-trip the hidden flag + reason,
// DELETE removes the row. Invalid bodies/reasons are field-level 422s.
func TestCommentModeration(t *testing.T) {
	srv, _ := newServer(t)
	id := createIssue(t, srv, `{"title":"t","body":"x"}`)
	base := fmt.Sprintf("/api/v1/w/default/issues/%d/comments", id)

	code, out := do(t, srv, "POST", base, `{"body":"first"}`)
	if code != http.StatusCreated {
		t.Fatalf("create comment = %d", code)
	}
	cid := int64(out["comment"].(map[string]any)["id"].(float64))
	path := fmt.Sprintf("%s/%d", base, cid)

	code, out = do(t, srv, "PATCH", path, `{"body":"edited"}`)
	if code != http.StatusOK {
		t.Fatalf("patch = %d %v", code, out)
	}
	c := out["comment"].(map[string]any)
	if c["body"] != "edited" || c["edited_at"] == nil {
		t.Fatalf("patched comment = %v, want body+edited_at", c)
	}

	// Empty body and bogus reason are 422s naming the field.
	code, out = do(t, srv, "PATCH", path, `{"body":"  "}`)
	if code != http.StatusUnprocessableEntity || out["error"].(map[string]any)["fields"].(map[string]any)["body"] == nil {
		t.Fatalf("empty body = %d %v, want 422 body field", code, out)
	}
	code, out = do(t, srv, "POST", path+"/hide", `{"reason":"rude"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("bad reason = %d %v, want 422", code, out)
	}
	if fields := out["error"].(map[string]any)["fields"].(map[string]any); fields["reason"] == nil {
		t.Fatalf("fields = %v, want reason", fields)
	}

	code, out = do(t, srv, "POST", path+"/hide", `{"reason":"spam"}`)
	if code != http.StatusOK {
		t.Fatalf("hide = %d %v", code, out)
	}
	c = out["comment"].(map[string]any)
	if c["hidden"] != true || c["hidden_reason"] != "spam" {
		t.Fatalf("hidden comment = %v", c)
	}

	code, out = do(t, srv, "POST", path+"/unhide", `{}`)
	if code != http.StatusOK {
		t.Fatalf("unhide = %d", code)
	}
	// hidden_reason is omitempty: restored rows omit it entirely.
	if c := out["comment"].(map[string]any); c["hidden"] != false || c["hidden_reason"] != nil {
		t.Fatalf("unhidden comment = %v", c)
	}

	// A comment is scoped to its issue: same cid under another issue 404s.
	other := createIssue(t, srv, `{"title":"other"}`)
	code, _ = do(t, srv, "PATCH", fmt.Sprintf("/api/v1/w/default/issues/%d/comments/%d", other, cid), `{"body":"x"}`)
	if code != http.StatusNotFound {
		t.Fatalf("cross-issue patch = %d, want 404", code)
	}

	// Workspace scope: the same comment under another workspace slug 404s.
	do(t, srv, "POST", "/api/v1/workspaces", `{"slug":"other"}`)
	for _, tc := range []struct {
		method, path, body string
	}{
		{"PATCH", fmt.Sprintf("/api/v1/w/other/issues/%d/comments/%d", id, cid), `{"body":"x"}`},
		{"DELETE", fmt.Sprintf("/api/v1/w/other/issues/%d/comments/%d", id, cid), ""},
		{"POST", fmt.Sprintf("/api/v1/w/other/issues/%d/comments/%d/hide", id, cid), `{"reason":"spam"}`},
	} {
		code, _ = do(t, srv, tc.method, tc.path, tc.body)
		if code != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404", tc.method, tc.path, code)
		}
	}

	code, _ = do(t, srv, "DELETE", path, "")
	if code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", code)
	}
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/issues/%d", id), "")
	if len(out["comments"].([]any)) != 0 {
		t.Fatalf("comments after delete = %v", out["comments"])
	}
}
