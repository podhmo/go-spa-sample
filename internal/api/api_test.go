// Handler tests for the /api/v1 JSON API. They run against a real SQLite
// store (fresh file DB per test) — the API is a thin JSON layer over the
// store, so the meaningful coverage is the wire contract: auth, status
// codes, envelope shapes, and per-field validation errors. The mount-time
// fail-closed behavior (no token → no /api/v1 at all) is covered by the
// web package's integration tests.
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/store"
)

const testToken = "test-token-1234"

// newServer boots the API handler over a fresh store and returns the
// server plus the store handle (needed to seed labels/categories that
// have no API endpoint).
func newServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(New(st, testToken, nil, nil))
	t.Cleanup(srv.Close)
	return srv, st
}

// do sends an authorized JSON request and returns status + decoded body.
func do(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != "" {
		rd = bytes.NewReader([]byte(body))
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if resp.ContentLength != 0 {
		json.NewDecoder(resp.Body).Decode(&out)
	}
	return resp.StatusCode, out
}

func TestAuthRequired(t *testing.T) {
	srv, _ := newServer(t)

	for _, auth := range []string{"", "Bearer wrong", "bearer " + testToken, "Basic " + testToken} {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/w/default/issues", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("Authorization %q → status %d, want 401", auth, resp.StatusCode)
		}
	}

	code, _ := do(t, srv, "GET", "/api/v1/w/default/issues", "")
	if code != http.StatusOK {
		t.Fatalf("authorized GET → %d, want 200", code)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/w/default/issues", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("401 Content-Type = %q, want application/json", ct)
	}
}

func TestWrongMethodAndTrailingJSON(t *testing.T) {
	srv, _ := newServer(t)

	req, _ := http.NewRequest("PATCH", srv.URL+"/api/v1/w/default/issues/1/status", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH status route = %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") != "POST" {
		t.Fatalf("Allow = %q, want POST", resp.Header.Get("Allow"))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("405 Content-Type = %q", ct)
	}

	// HEAD rides on GET like the std mux's method-scoped patterns.
	req, _ = http.NewRequest("HEAD", srv.URL+"/api/v1/w/default/issues", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD = %d, want 200", resp.StatusCode)
	}

	code, _ := do(t, srv, "POST", "/api/v1/w/default/issues", `{"title":"x"} {"title":"y"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("trailing payload = %d, want 400", code)
	}
	code, _ = do(t, srv, "POST", "/api/v1/w/default/issues", `{"title":"x"}garbage`)
	if code != http.StatusBadRequest {
		t.Fatalf("trailing garbage = %d, want 400", code)
	}
}

func TestNotFoundAndUnknownRoutes(t *testing.T) {
	srv, _ := newServer(t)

	for path, code := range map[string]int{
		"/api/v1/w/default/issues/999":    http.StatusNotFound,
		"/api/v1/w/default/issues/abc":    http.StatusNotFound,
		"/api/v1/nope":                    http.StatusNotFound,
		"/api/v1/w/default/discussions/7": http.StatusNotFound,
	} {
		got, out := do(t, srv, "GET", path, "")
		if got != code {
			t.Fatalf("GET %s = %d, want %d", path, got, code)
		}
		if out["error"].(map[string]any)["message"] == nil {
			t.Fatalf("GET %s error body = %v", path, out)
		}
	}
}

// Per-user api_tokens resolve the Bearer token to a user: writes carry
// that user's author_id, while the shared env token stays anonymous.
func TestPerUserTokenActor(t *testing.T) {
	srv, st := newServer(t)
	ctx := t.Context()

	uid, err := st.CreateUser(ctx, "erin", "irrelevant-hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	userToken, err := st.CreateAPIToken(ctx, uid)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	post := func(token string) map[string]any {
		t.Helper()
		req, err := http.NewRequest("POST", srv.URL+"/api/v1/w/default/issues",
			bytes.NewReader([]byte(`{"title":"actor test"}`)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("POST with user token → %d, want 201: %v", resp.StatusCode, out)
		}
		return out
	}

	out := post(testToken)
	if issue, _ := out["issue"].(map[string]any); issue["author"] != nil {
		t.Errorf("env-token issue author = %v, want anonymous", issue["author"])
	}

	out = post(userToken)
	issue, _ := out["issue"].(map[string]any)
	author, _ := issue["author"].(map[string]any)
	if author["name"] != "erin" {
		t.Errorf("user-token issue author = %v, want erin", author)
	}

	// Unknown and revoked tokens are 401, and revoking takes effect at
	// request time (no restart needed once the API is mounted).
	for _, token := range []string{"no-such-token", "wrong"} {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/w/default/issues", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("Bearer %q → %d, want 401", token, resp.StatusCode)
		}
	}
	if err := st.DeleteAPIToken(ctx, userToken); err != nil {
		t.Fatalf("DeleteAPIToken: %v", err)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/w/default/issues", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token → %d, want 401", resp.StatusCode)
	}
}
