package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doText sends an authorized request and returns status + response
// headers + raw body — the non-JSON counterpart of do() for the markdown
// endpoints.
func doText(t *testing.T, srv *httptest.Server, method, path, body string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, string(raw)
}

// createIssue POSTs an issue and returns its id.
func createIssue(t *testing.T, srv *httptest.Server, body string) int64 {
	t.Helper()
	code, out := do(t, srv, "POST", "/api/v1/w/default/issues", body)
	if code != http.StatusCreated {
		t.Fatalf("POST /api/v1/issues status = %d, want 201 (body %v)", code, out)
	}
	return int64(out["issue"].(map[string]any)["id"].(float64))
}

func TestIssueLifecycle(t *testing.T) {
	srv, _ := newServer(t)

	code, out := do(t, srv, "POST", "/api/v1/w/default/issues", `{"title":"first","body":"hello"}`)
	if code != http.StatusCreated {
		t.Fatalf("create status = %d (%v)", code, out)
	}
	issue := out["issue"].(map[string]any)
	if issue["status"] != "open" || issue["title"] != "first" {
		t.Fatalf("unexpected issue DTO: %v", issue)
	}
	id := int64(issue["id"].(float64))

	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/issues/%d", id), "")
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	if cs, ok := out["comments"].([]any); !ok || len(cs) != 0 {
		t.Fatalf("comments = %v, want empty array", out["comments"])
	}

	code, out = do(t, srv, "GET", "/api/v1/w/default/issues?status=open&q=first", "")
	if code != http.StatusOK || out["total"].(float64) != 1 {
		t.Fatalf("list = %d %v", code, out)
	}

	code, out = do(t, srv, "PATCH", fmt.Sprintf("/api/v1/w/default/issues/%d", id), `{"title":"renamed"}`)
	if code != http.StatusOK {
		t.Fatalf("patch status = %d (%v)", code, out)
	}
	if got := out["issue"].(map[string]any); got["title"] != "renamed" || got["body"] != "hello" {
		t.Fatalf("patch lost body: %v", got)
	}

	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/issues/%d/status", id), `{"status":"closed"}`)
	if code != http.StatusOK || out["issue"].(map[string]any)["status"] != "closed" {
		t.Fatalf("status = %d %v", code, out)
	}
	code, out = do(t, srv, "GET", "/api/v1/w/default/issues?status=open", "")
	if out["total"].(float64) != 0 {
		t.Fatalf("open list should be empty: %v", out)
	}

	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/issues/%d/comments", id), `{"body":"me too"}`)
	if code != http.StatusCreated || out["comment"].(map[string]any)["body"] != "me too" {
		t.Fatalf("comment = %d %v", code, out)
	}
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/issues/%d", id), "")
	if n := len(out["comments"].([]any)); n != 1 {
		t.Fatalf("comments = %d, want 1", n)
	}

	code, _ = do(t, srv, "DELETE", fmt.Sprintf("/api/v1/w/default/issues/%d", id), "")
	if code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", code)
	}
	code, _ = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/issues/%d", id), "")
	if code != http.StatusNotFound {
		t.Fatalf("deleted issue = %d, want 404", code)
	}
}

// The markdown endpoint answers text/markdown: the issue plus every
// comment as one document. Errors stay JSON.
func TestIssueMarkdown(t *testing.T) {
	srv, _ := newServer(t)
	id := createIssue(t, srv, `{"title":"first","body":"hello"}`)
	do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/issues/%d/comments", id), `{"body":"me too"}`)

	code, hdr, body := doText(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/issues/%d/markdown", id), "")
	if code != http.StatusOK {
		t.Fatalf("markdown status = %d", code)
	}
	if ct := hdr.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/markdown", ct)
	}
	// inline, not attachment: the body is clipboard input, not a download.
	if cd := hdr.Get("Content-Disposition"); cd != fmt.Sprintf(`inline; filename="issue-%d.md"`, id) {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	for _, want := range []string{"# first\n", "- **Status**: open\n", "hello", "## Comments", "### Comment by anonymous", "me too"} {
		if !strings.Contains(body, want) {
			t.Errorf("markdown missing %q:\n%s", want, body)
		}
	}

	// Missing issue → JSON 404 envelope, not a markdown body.
	code, out := do(t, srv, "GET", "/api/v1/w/default/issues/999/markdown", "")
	if code != http.StatusNotFound || out["error"] == nil {
		t.Fatalf("missing issue = %d %v, want 404 JSON", code, out)
	}

	// An issue in "default" is invisible from another workspace slug.
	do(t, srv, "POST", "/api/v1/workspaces", `{"slug":"other"}`)
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/other/issues/%d/markdown", id), "")
	if code != http.StatusNotFound || out["error"] == nil {
		t.Fatalf("cross-workspace markdown = %d %v, want 404 JSON", code, out)
	}

	// POST is a JSON 405 with Allow: GET.
	req, _ := http.NewRequest("POST", srv.URL+fmt.Sprintf("/api/v1/w/default/issues/%d/markdown", id), nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST markdown = %d Allow=%q, want 405 GET, HEAD", resp.StatusCode, resp.Header.Get("Allow"))
	}
}

func TestCrossWorkspaceScopeHint(t *testing.T) {
	srv, _ := newServer(t)

	_, out := do(t, srv, "POST", "/api/v1/w/default/issues", `{"title":"x"}`)
	id := int64(out["issue"].(map[string]any)["id"].(float64))
	do(t, srv, "POST", "/api/v1/workspaces", `{"slug":"other"}`)

	// GET on the wrong workspace: the 404 names the real slug so a
	// client can rebase onto /w/{slug}/... itself.
	code, out := do(t, srv, "GET", fmt.Sprintf("/api/v1/w/other/issues/%d", id), "")
	if code != http.StatusNotFound {
		t.Fatalf("cross-ws GET = %d, want 404", code)
	}
	if got := out["error"].(map[string]any)["workspace"]; got != "default" {
		t.Fatalf("cross-ws GET error.workspace = %v, want %q", got, "default")
	}

	// Mutations stay a bare 404 — no hint inviting a replay elsewhere.
	code, out = do(t, srv, "PATCH", fmt.Sprintf("/api/v1/w/other/issues/%d", id), `{"title":"y"}`)
	if code != http.StatusNotFound {
		t.Fatalf("cross-ws PATCH = %d, want 404", code)
	}
	if _, ok := out["error"].(map[string]any)["workspace"]; ok {
		t.Fatalf("cross-ws PATCH must not carry error.workspace: %v", out)
	}

	// HEAD strips the body, so the slug also rides the X-Workspace-Slug
	// header (on GETs too — one code path).
	req, _ := http.NewRequest("HEAD", srv.URL+fmt.Sprintf("/api/v1/w/other/issues/%d", id), nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get("X-Workspace-Slug") != "default" {
		t.Fatalf("cross-ws HEAD = %d X-Workspace-Slug=%q, want 404 default",
			resp.StatusCode, resp.Header.Get("X-Workspace-Slug"))
	}
}

func TestIssueValidation(t *testing.T) {
	srv, _ := newServer(t)

	code, out := do(t, srv, "POST", "/api/v1/w/default/issues", `{"title":"  ","body":"x"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%v)", code, out)
	}
	fields := out["error"].(map[string]any)["fields"].(map[string]any)
	if fields["title"] == nil {
		t.Fatalf("fields = %v, want title error", fields)
	}

	code, _ = do(t, srv, "POST", "/api/v1/w/default/issues", `{"title":`)
	if code != http.StatusBadRequest {
		t.Fatalf("bad json = %d, want 400", code)
	}

	code, out = do(t, srv, "POST", "/api/v1/w/default/issues", `{"title":"t","labels":["nope"]}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown label = %d", code)
	}
	if out["error"].(map[string]any)["fields"].(map[string]any)["labels"] == nil {
		t.Fatalf("fields = %v, want labels error", out)
	}

	code, out = do(t, srv, "GET", "/api/v1/w/default/labels", "")
	if code != http.StatusOK {
		t.Fatalf("labels = %d", code)
	}
	if _, ok := out["labels"].([]any); !ok {
		t.Fatalf("labels payload = %v", out)
	}

	id := createIssue(t, srv, `{"title":"t"}`)
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/issues/%d/status", id), `{"status":"wip"}`)
	if code != http.StatusUnprocessableEntity || out["error"].(map[string]any)["fields"].(map[string]any)["status"] == nil {
		t.Fatalf("status = %d %v", code, out)
	}
}
