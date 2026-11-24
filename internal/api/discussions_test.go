package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// seedCategories makes the fixed discussion categories exist on the test
// store (they are master data the binary seeds at startup, not schema).
func seedCategories(t *testing.T, st *store.Store) int64 {
	t.Helper()
	if err := st.SeedCategories(t.Context()); err != nil {
		t.Fatalf("SeedCategories: %v", err)
	}
	cats, err := st.ListDiscussionCategories(t.Context())
	if err != nil || len(cats) == 0 {
		t.Fatalf("ListDiscussionCategories: %v (%d)", err, len(cats))
	}
	return cats[0].ID
}

// qaCategoryID returns the id of the answerable category.
func qaCategoryID(t *testing.T, st *store.Store) int64 {
	t.Helper()
	cats, err := st.ListDiscussionCategories(t.Context())
	if err != nil {
		t.Fatalf("ListDiscussionCategories: %v", err)
	}
	for _, c := range cats {
		if c.Format == store.CategoryFormatQA {
			return c.ID
		}
	}
	t.Fatal("no qa category seeded")
	return 0
}

func mustDiscussion(t *testing.T, srv *httptest.Server, catID int64, title string) map[string]any {
	t.Helper()
	code, out := do(t, srv, "POST", "/api/v1/w/default/discussions",
		fmt.Sprintf(`{"title":%q,"body":"b","category_id":%d}`, title, catID))
	if code != http.StatusCreated {
		t.Fatalf("POST discussion %q = %d: %v", title, code, out)
	}
	return out["discussion"].(map[string]any)
}

func TestDiscussionCategories(t *testing.T) {
	srv, st := newServer(t)
	seedCategories(t, st)

	code, out := do(t, srv, "GET", "/api/v1/discussion-categories", "")
	if code != http.StatusOK {
		t.Fatalf("GET categories = %d, want 200", code)
	}
	cats := out["categories"].([]any)
	if len(cats) < 4 {
		t.Fatalf("categories = %d, want >= 4", len(cats))
	}
	var qa map[string]any
	for _, c := range cats {
		m := c.(map[string]any)
		if m["slug"] == "qa" {
			qa = m
		}
	}
	if qa == nil || qa["format"] != store.CategoryFormatQA {
		t.Fatalf("qa category = %v, want format qa", qa)
	}
}

func TestDiscussionLifecycle(t *testing.T) {
	srv, st := newServer(t)
	seedCategories(t, st)

	// missing category is a per-field 422
	code, out := do(t, srv, "POST", "/api/v1/w/default/discussions", `{"title":"t","body":"b"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("POST without category = %d, want 422", code)
	}
	if out["error"].(map[string]any)["fields"].(map[string]any)["category"] == nil {
		t.Fatalf("category field error missing: %v", out)
	}

	// create via category slug
	code, out = do(t, srv, "POST", "/api/v1/w/default/discussions",
		`{"title":"hello","body":"world","category":"general"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST discussion = %d, want 201: %v", code, out)
	}
	d := out["discussion"].(map[string]any)
	did := int64(d["id"].(float64))
	if d["category"].(map[string]any)["slug"] != "general" {
		t.Fatalf("category = %v, want general", d["category"])
	}
	if d["answered"] != false {
		t.Fatalf("answered = %v, want false", d["answered"])
	}

	// list filter: answered=no matches (no answer yet)
	code, out = do(t, srv, "GET", "/api/v1/w/default/discussions?answered=no", "")
	if code != http.StatusOK || out["total"].(float64) != 1 {
		t.Fatalf("answered=no = %d total %v, want 200/1", code, out["total"])
	}
	code, out = do(t, srv, "GET", "/api/v1/w/default/discussions?answered=yes", "")
	if code != http.StatusOK || out["total"].(float64) != 0 {
		t.Fatalf("answered=yes = %d total %v, want 200/0", code, out["total"])
	}
	code, out = do(t, srv, "GET", "/api/v1/w/default/discussions?q=hell", "")
	if code != http.StatusOK || out["total"].(float64) != 1 {
		t.Fatalf("q=hell = %d total %v, want 200/1", code, out["total"])
	}

	// a thread, a reply, then the reply is edited
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads", did), `{"body":"op"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST thread = %d, want 201: %v", code, out)
	}
	tid := int64(out["thread"].(map[string]any)["id"].(float64))

	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/comments", did, tid), `{"body":"reply"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST reply = %d, want 201: %v", code, out)
	}
	cid := int64(out["reply"].(map[string]any)["id"].(float64))

	code, out = do(t, srv, "PATCH", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/comments/%d", did, tid, cid), `{"body":"reply v2"}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH reply = %d, want 200: %v", code, out)
	}
	if out["reply"].(map[string]any)["edited_at"] == nil {
		t.Fatalf("reply edited_at = %v, want set", out["reply"])
	}

	// open-format categories cannot take an answer mark
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/answer", did, tid), `{"answer":true}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("answer on open category = %d, want 422", code)
	}

	// hide + unhide the reply
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/comments/%d/hide", did, tid, cid), `{"reason":"outdated"}`)
	if code != http.StatusOK || out["reply"].(map[string]any)["hidden"] != true {
		t.Fatalf("hide reply = %d hidden=%v", code, out["reply"])
	}
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/comments/%d/unhide", did, tid, cid), `{}`)
	if code != http.StatusOK || out["reply"].(map[string]any)["hidden"] != false {
		t.Fatalf("unhide reply = %d hidden=%v", code, out["reply"])
	}

	// delete the reply, then the thread
	code, _ = do(t, srv, "DELETE", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/comments/%d", did, tid, cid), "")
	if code != http.StatusNoContent {
		t.Fatalf("DELETE reply = %d, want 204", code)
	}
	code, _ = do(t, srv, "DELETE", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d", did, tid), "")
	if code != http.StatusNoContent {
		t.Fatalf("DELETE thread = %d, want 204", code)
	}
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/discussions/%d", did), "")
	if code != http.StatusOK || len(out["threads"].([]any)) != 0 {
		t.Fatalf("threads after delete = %v", out["threads"])
	}

	// PATCH the discussion itself: move to qa + new title
	code, out = do(t, srv, "PATCH", fmt.Sprintf("/api/v1/w/default/discussions/%d", did), `{"title":"moved","category":"qa"}`)
	if code != http.StatusOK || out["discussion"].(map[string]any)["category"].(map[string]any)["slug"] != "qa" {
		t.Fatalf("PATCH discussion = %d: %v", code, out)
	}

	// markdown export renders the title
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/discussions/%d/markdown", did), "")
	if code != http.StatusOK {
		t.Fatalf("GET markdown = %d, want 200", code)
	}
}

func TestDiscussionAnswerMark(t *testing.T) {
	srv, st := newServer(t)
	seedCategories(t, st)
	qaID := qaCategoryID(t, st)

	code, out := do(t, srv, "POST", "/api/v1/w/default/discussions",
		fmt.Sprintf(`{"title":"how","body":"?","category_id":%d}`, qaID))
	if code != http.StatusCreated {
		t.Fatalf("POST qa discussion = %d: %v", code, out)
	}
	did := int64(out["discussion"].(map[string]any)["id"].(float64))

	var tids [2]int64
	for i := range tids {
		code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads", did), fmt.Sprintf(`{"body":"t%d"}`, i))
		if code != http.StatusCreated {
			t.Fatalf("POST thread %d = %d", i, code)
		}
		tids[i] = int64(out["thread"].(map[string]any)["id"].(float64))
	}

	// mark thread 1 as the answer → discussion reports answered
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/answer", did, tids[0]), `{"answer":true}`)
	if code != http.StatusOK || out["thread"].(map[string]any)["is_answer"] != true {
		t.Fatalf("mark answer = %d: %v", code, out)
	}
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/discussions/%d", did), "")
	if code != http.StatusOK || out["discussion"].(map[string]any)["answered"] != true {
		t.Fatalf("answered = %v, want true", out["discussion"])
	}

	// marking thread 2 moves the mark — thread 1 must come out unmarked
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/answer", did, tids[1]), `{"answer":true}`)
	if code != http.StatusOK {
		t.Fatalf("move answer = %d: %v", code, out)
	}
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/default/discussions/%d", did), "")
	if code != http.StatusOK || out["discussion"].(map[string]any)["answered"] != true {
		t.Fatalf("answered = %v, want true after move", out["discussion"])
	}
	got := map[int64]bool{}
	for _, t0 := range out["threads"].([]any) {
		m := t0.(map[string]any)
		got[int64(m["id"].(float64))] = m["is_answer"] == true
	}
	for _, tid := range tids {
		if _, ok := got[tid]; !ok {
			t.Fatalf("thread %d missing from detail: %v", tid, got)
		}
	}
	if got[tids[0]] || !got[tids[1]] {
		t.Fatalf("is_answer = %v, want only thread %d marked", got, tids[1])
	}
}

func TestDiscussionPatchClearLabels(t *testing.T) {
	srv, st := newServer(t)
	catID := seedCategories(t, st)
	ctx := t.Context()
	ws, err := st.WorkspaceBySlug(ctx, "default")
	if err != nil {
		t.Fatalf("WorkspaceBySlug: %v", err)
	}
	lid, err := st.CreateLabel(ctx, ws.ID, store.KindLabelDiscussion, store.LabelInput{Name: "meta", Color: "#aaaaaa"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}

	code, out := do(t, srv, "POST", "/api/v1/w/default/discussions",
		fmt.Sprintf(`{"title":"d","body":"b","category_id":%d,"label_ids":[%d]}`, catID, lid))
	if code != http.StatusCreated {
		t.Fatalf("POST = %d: %v", code, out)
	}
	did := int64(out["discussion"].(map[string]any)["id"].(float64))
	if len(out["discussion"].(map[string]any)["labels"].([]any)) != 1 {
		t.Fatalf("labels = %v, want 1", out["discussion"])
	}

	// an explicitly empty "labels" array clears — nil would mean "leave as is"
	code, out = do(t, srv, "PATCH", fmt.Sprintf("/api/v1/w/default/discussions/%d", did), `{"labels":[]}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH labels:[] = %d: %v", code, out)
	}
	if got := out["discussion"].(map[string]any)["labels"].([]any); len(got) != 0 {
		t.Fatalf("labels after clear = %v, want empty", got)
	}
}

func TestDiscussionCrossScopeAndThreadGuards(t *testing.T) {
	srv, st := newServer(t)
	catID := seedCategories(t, st)
	ctx := t.Context()
	if _, err := st.CreateWorkspace(ctx, store.WorkspaceInput{Slug: "other"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	did := int64(mustDiscussion(t, srv, catID, "d1")["id"].(float64))
	code, out := do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads", did), `{"body":"op"}`)
	if code != http.StatusCreated {
		t.Fatalf("thread = %d", code)
	}
	tid := int64(out["thread"].(map[string]any)["id"].(float64))

	// GET under a foreign slug → 404 carrying the real workspace hint
	code, out = do(t, srv, "GET", fmt.Sprintf("/api/v1/w/other/discussions/%d", did), "")
	if code != http.StatusNotFound {
		t.Fatalf("cross-scope GET = %d, want 404", code)
	}
	if out["error"].(map[string]any)["workspace"] != "default" {
		t.Fatalf("workspace hint = %v, want default", out["error"])
	}

	// mutations get a bare 404 — no hint invites a replay elsewhere
	code, out = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/other/discussions/%d/threads", did), `{"body":"x"}`)
	if code != http.StatusNotFound {
		t.Fatalf("cross-scope POST = %d, want 404", code)
	}
	if _, ok := out["error"].(map[string]any)["workspace"]; ok {
		t.Fatalf("mutation 404 leaked workspace hint: %v", out["error"])
	}

	// a thread id that belongs to another discussion is a 404, not a reply
	did2 := int64(mustDiscussion(t, srv, catID, "d2")["id"].(float64))
	code, _ = do(t, srv, "POST", fmt.Sprintf("/api/v1/w/default/discussions/%d/threads/%d/comments", did2, tid), `{"body":"x"}`)
	if code != http.StatusNotFound {
		t.Fatalf("foreign thread reply = %d, want 404", code)
	}

	// wrong methods on discussion routes → 405 with Allow
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/w/default/discussions", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE discussions = %d, want 405", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Allow"), "GET") {
		t.Fatalf("Allow = %q, want GET listed", resp.Header.Get("Allow"))
	}
}
