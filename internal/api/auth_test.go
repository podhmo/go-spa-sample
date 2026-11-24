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

// The public /auth/* endpoints are exercised here without any bearer
// credential — they are how one is obtained.
func TestAuthFlow(t *testing.T) {
	srv, _ := newServer(t)

	post := func(path, body string, jar []*http.Cookie) (*http.Response, map[string]any) {
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader([]byte(body)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		for _, c := range jar {
			req.AddCookie(c)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		var out map[string]any
		if resp.ContentLength != 0 {
			json.NewDecoder(resp.Body).Decode(&out)
		}
		resp.Body.Close()
		return resp, out
	}
	sessionCookie := func(resp *http.Response) *http.Cookie {
		t.Helper()
		for _, c := range resp.Cookies() {
			if c.Name == sessionCookieName && c.Value != "" {
				return c
			}
		}
		t.Fatal("no session cookie in response")
		return nil
	}

	// register → 201 + session cookie.
	resp, body := post("/api/v1/auth/register", `{"name":"alice","password":"hunter22ok"}`, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("register status = %d, body %v", resp.StatusCode, body)
	}
	if u, _ := body["user"].(map[string]any); u == nil || u["name"] != "alice" {
		t.Fatalf("register user = %v", body["user"])
	}
	cookie := sessionCookie(resp)

	// The session cookie authenticates API calls and wins over the dev
	// shared token: /auth/me must report alice, not anonymous.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/me", nil)
	req.AddCookie(cookie)
	req.Header.Set("Authorization", "Bearer "+testToken)
	meResp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var me map[string]any
	json.NewDecoder(meResp.Body).Decode(&me)
	meResp.Body.Close()
	if meResp.StatusCode != 200 {
		t.Fatalf("me status = %d, body %v", meResp.StatusCode, me)
	}
	if u := me["user"].(map[string]any); u["name"] != "alice" {
		t.Fatalf("me user = %v", u)
	}

	// Register validation: duplicate name (case-insensitive), short
	// password, missing fields each land a 422 field error.
	cases := []struct {
		body  string
		field string
	}{
		{`{"name":"ALICE","password":"hunter22ok"}`, "name"},
		{`{"name":"bob","password":"short"}`, "password"},
		{`{"name":"bob"}`, "password"},
		{`{"password":"hunter22ok"}`, "name"},
	}
	for _, tc := range cases {
		resp, body := post("/api/v1/auth/register", tc.body, nil)
		if resp.StatusCode != 422 {
			t.Fatalf("register %s status = %d", tc.body, resp.StatusCode)
		}
		fields := body["error"].(map[string]any)["fields"].(map[string]any)
		if fields[tc.field] == nil {
			t.Fatalf("register %s fields = %v", tc.body, fields)
		}
	}

	// Login: bad credentials get the vague 422, good ones a session.
	resp, body = post("/api/v1/auth/login", `{"name":"alice","password":"wrong-password"}`, nil)
	if resp.StatusCode != 422 {
		t.Fatalf("bad login status = %d", resp.StatusCode)
	}
	if _, has := body["error"].(map[string]any)["fields"]; has {
		t.Fatalf("login failure must not name the failing field: %v", body)
	}
	resp, _ = post("/api/v1/auth/login", `{"name":"ALICE","password":"hunter22ok"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	loginCookie := sessionCookie(resp)

	// logout kills the session row and expires the cookie.
	resp, _ = post("/api/v1/auth/logout", `{}`, []*http.Cookie{loginCookie})
	if resp.StatusCode != 204 {
		t.Fatalf("logout status = %d", resp.StatusCode)
	}
	var expired bool
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.MaxAge < 0 {
			expired = true
		}
	}
	if !expired {
		t.Fatal("logout did not expire the session cookie")
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/me", nil)
	req.AddCookie(loginCookie)
	meResp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	meResp.Body.Close()
	if meResp.StatusCode != 401 {
		t.Fatalf("me after logout status = %d", meResp.StatusCode)
	}

	// Anonymous shared-token requests have no identity for /auth/me.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	meResp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	meResp.Body.Close()
	if meResp.StatusCode != 401 {
		t.Fatalf("me anonymous status = %d", meResp.StatusCode)
	}
}

// No env token, no api_tokens rows: the API still mounts and a session
// cookie alone carries requests through the credential gate.
func TestTokenlessStartup(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(New(st, "", nil))
	t.Cleanup(srv.Close)

	// Anonymous calls are rejected — there is no shared token to borrow.
	resp, err := srv.Client().Get(srv.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("anonymous me status = %d", resp.StatusCode)
	}

	// Register → the session cookie alone authenticates /auth/me.
	resp, err = srv.Client().Post(srv.URL+"/api/v1/auth/register",
		"application/json", bytes.NewReader([]byte(`{"name":"solo","password":"hunter22ok"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("register status = %d", resp.StatusCode)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			cookie = c
		}
	}
	resp.Body.Close()
	if cookie == nil {
		t.Fatal("no session cookie in response")
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/me", nil)
	req.AddCookie(cookie)
	meResp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	meResp.Body.Close()
	if meResp.StatusCode != 200 {
		t.Fatalf("cookie me status = %d", meResp.StatusCode)
	}
}

// The public /auth/* endpoints only take application/json. A hostile
// site can only emit a "simple" cross-origin POST — form or text/plain,
// even with a JSON body — so without this check login CSRF could plant
// an attacker-chosen session in a visitor's browser.
func TestAuthRequiresJSONContentType(t *testing.T) {
	srv, _ := newServer(t)
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login",
			bytes.NewReader([]byte(`{"name":"x","password":"y"}`)))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 415 {
			t.Fatalf("login with Content-Type %q: status = %d, want 415", ct, resp.StatusCode)
		}
	}
	// ...but it must tolerate JSON's case-insensitive media type.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login",
		bytes.NewReader([]byte(`{"name":"x","password":"y"}`)))
	req.Header.Set("Content-Type", "Application/JSON; charset=utf-8")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == 415 {
		t.Fatal("Application/JSON was rejected")
	}

	// logout takes no body — a plain POST must reach it.
	resp, err = srv.Client().Post(srv.URL+"/api/v1/auth/logout", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == 415 {
		t.Fatal("bodyless logout was rejected")
	}
}
