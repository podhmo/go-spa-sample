// /api/v1/pulls* tests: the same store+repo plumbing as the web tests,
// checking the JSON wire contract — 404 when no repo is mounted,
// create/list/get, comments, status transitions, and merge.
package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/gitrepo"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// newPullServer boots the API over a store + a real git repo (main with
// one commit, feat/x branched off it) and returns everything a test
// needs.
func newPullServer(t *testing.T) (*httptest.Server, *store.Store, *gitrepo.Repo) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "tester")
	for _, f := range []struct{ name, content string }{{"a.txt", "one\n"}} {
		p := filepath.Join(dir, f.name)
		if err := writeTestFile(p, f.content); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "a.txt")
	git("commit", "-m", "initial")
	git("checkout", "-b", "feat/x")
	if err := writeTestFile(filepath.Join(dir, "b.txt"), "two\nthree\n"); err != nil {
		t.Fatal(err)
	}
	git("add", "b.txt")
	git("commit", "-m", "add b")

	repo, err := gitrepo.Resolve(context.Background(), gitrepo.Options{RepoPath: dir})
	if err != nil || repo == nil {
		t.Fatalf("gitrepo.Resolve: repo=%v err=%v", repo, err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(New(st, testToken, repo, nil))
	t.Cleanup(srv.Close)
	return srv, st, repo
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// doRaw sends an authorized request and returns status + raw body —
// /diff and /markdown answer text, not JSON.
func doRaw(t *testing.T, srv *httptest.Server, method, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func TestPullsAPI_NoRepo(t *testing.T) {
	srv, _ := newServer(t) // repo nil — pulls are not mounted
	status, _ := do(t, srv, "GET", "/api/v1/pulls", "")
	if status != http.StatusNotFound {
		t.Fatalf("GET /pulls without repo: got %d, want 404", status)
	}
}

func TestPullsAPI_CreateListGet(t *testing.T) {
	srv, _, _ := newPullServer(t)
	status, body := do(t, srv, "POST", "/api/v1/pulls",
		`{"base":"main","head":"feat/x","title":"add b","body":"adds b.txt"}`)
	if status != http.StatusCreated {
		t.Fatalf("POST /pulls: got %d %v", status, body)
	}
	pr := body["pull_request"].(map[string]any)
	id := int64(pr["id"].(float64))
	if pr["base"] != "refs/heads/main" || pr["head"] != "refs/heads/feat/x" || pr["status"] != "open" {
		t.Fatalf("unexpected pull body: %v", pr)
	}
	status, list := do(t, srv, "GET", "/api/v1/pulls", "")
	if status != http.StatusOK {
		t.Fatalf("GET /pulls: %d", status)
	}
	if got := len(list["pulls"].([]any)); got != 1 {
		t.Fatalf("list: got %d pulls", got)
	}

	status, got := do(t, srv, "GET", fmt.Sprintf("/api/v1/pulls/%d", id), "")
	if status != http.StatusOK {
		t.Fatalf("GET /pulls/%d: %d", id, status)
	}
	if got["commits"] == nil || len(got["commits"].([]any)) != 1 {
		t.Fatalf("want 1 commit in view, got %v", got["commits"])
	}
	if m, ok := got["mergeable"].(map[string]any); !ok || m["status"] != "clean" {
		t.Fatalf("mergeable: got %v", got["mergeable"])
	}
}

func TestPullsAPI_CreateValidation(t *testing.T) {
	srv, _, _ := newPullServer(t)
	status, body := do(t, srv, "POST", "/api/v1/pulls",
		`{"base":"main","head":"feat/x","title":""}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty title: got %d, want 422", status)
	}
	if fe, ok := body["error"].(map[string]any)["fields"].(map[string]any); !ok || fe["title"] == nil {
		t.Fatalf("want error.fields.title in 422 body, got %v", body)
	}
	status, body = do(t, srv, "POST", "/api/v1/pulls",
		`{"base":"main","head":"main","title":"self"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("same base/head: got %d, want 422", status)
	}
	_ = body
}

func TestPullsAPI_CommentsAndStatus(t *testing.T) {
	srv, _, _ := newPullServer(t)
	_, body := do(t, srv, "POST", "/api/v1/pulls",
		`{"base":"main","head":"feat/x","title":"add b"}`)
	id := int64(body["pull_request"].(map[string]any)["id"].(float64))

	status, c := do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/comments", id),
		`{"body":"conversation"}`)
	if status != http.StatusCreated {
		t.Fatalf("POST comment: %d %v", status, c)
	}
	c = c["comment"].(map[string]any)
	status, rc := do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/comments", id),
		`{"body":"anchored","path":"b.txt","line":1,"side":"right"}`)
	if status != http.StatusCreated {
		t.Fatalf("anchored comment: %d %v", status, rc)
	}
	rcc := rc["comment"].(map[string]any)
	if rcc["anchor_sha"] == nil || rcc["anchor_sha"] == "" {
		t.Fatalf("want anchor in review comment, got %v", rc)
	}
	// Anchoring to a file the PR does not touch is rejected — accepting it
	// would leak arbitrary repo files through diff_snippet.
	status, bad := do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/comments", id),
		`{"body":"foreign","path":"a.txt","line":1,"side":"right"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("foreign-path anchor: got %d %v, want 422", status, bad)
	}

	cid := int64(c["id"].(float64))
	status, _ = do(t, srv, "PATCH", fmt.Sprintf("/api/v1/pulls/%d/comments/%d", id, cid), `{"body":"edited"}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH comment: %d", status)
	}
	status, _ = do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/comments/%d/hide", id, cid), `{"reason":"spam"}`)
	if status != http.StatusOK {
		t.Fatalf("hide: %d", status)
	}
	status, list := do(t, srv, "GET", fmt.Sprintf("/api/v1/pulls/%d/comments", id), "")
	if status != http.StatusOK || len(list["comments"].([]any)) != 2 {
		t.Fatalf("comments list: %d %v", status, list)
	}

	status, _ = do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/status", id), `{"status":"closed"}`)
	if status != http.StatusOK {
		t.Fatalf("close: %d", status)
	}
	status, _ = do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/status", id), `{"status":"open"}`)
	if status != http.StatusOK {
		t.Fatalf("reopen: %d", status)
	}
	status, _ = do(t, srv, "DELETE", fmt.Sprintf("/api/v1/pulls/%d", id), "")
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}
	status, _ = do(t, srv, "GET", fmt.Sprintf("/api/v1/pulls/%d", id), "")
	if status != http.StatusNotFound {
		t.Fatalf("get after delete: %d", status)
	}
}

func TestPullsAPI_Merge(t *testing.T) {
	srv, _, _ := newPullServer(t)
	_, body := do(t, srv, "POST", "/api/v1/pulls",
		`{"base":"main","head":"feat/x","title":"add b"}`)
	id := int64(body["pull_request"].(map[string]any)["id"].(float64))
	status, res := do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/merge", id), `{"method":"squash"}`)
	if status != http.StatusOK {
		t.Fatalf("merge: %d %v", status, res)
	}
	mp, _ := res["pull_request"].(map[string]any)
	if mp == nil || mp["status"] != "merged" || mp["merge_sha"] == nil || mp["merge_sha"] == "" {
		t.Fatalf("merged row: %v", res)
	}
	// Second merge is a conflict — the PR is not open anymore.
	status, _ = do(t, srv, "POST", fmt.Sprintf("/api/v1/pulls/%d/merge", id), `{"method":"squash"}`)
	if status != http.StatusConflict {
		t.Fatalf("re-merge: got %d, want 409", status)
	}
}

func TestPullsAPI_DiffAndMarkdown(t *testing.T) {
	srv, _, _ := newPullServer(t)
	_, body := do(t, srv, "POST", "/api/v1/pulls",
		`{"base":"main","head":"feat/x","title":"add b"}`)
	id := int64(body["pull_request"].(map[string]any)["id"].(float64))

	status, text := doRaw(t, srv, "GET", fmt.Sprintf("/api/v1/pulls/%d/diff", id))
	if status != http.StatusOK || !strings.Contains(text, "+three") {
		t.Fatalf("diff: %d %q", status, text[:min(len(text), 200)])
	}
	status, md := doRaw(t, srv, "GET", fmt.Sprintf("/api/v1/pulls/%d/markdown", id))
	if status != http.StatusOK || !strings.Contains(md, "# add b") {
		t.Fatalf("markdown: %d %q", status, md[:min(len(md), 200)])
	}
}
