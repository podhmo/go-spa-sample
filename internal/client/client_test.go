package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/podhmo/go-spa-sample/internal/api"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// newFixture stands up a real API (real store in a temp DB) behind httptest
// and returns a client pointed at it — the remote mode path end to end.
func newFixture(t *testing.T, token string) (*Client, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(api.New(st, token, nil))
	t.Cleanup(srv.Close)
	return New(srv.URL, token).In("default"), st
}

func TestClientIssueLifecycle(t *testing.T) {
	c, _ := newFixture(t, "env-tok")

	id, err := c.CreateIssue(context.Background(), store.IssueInput{
		Title: "via client", Body: "body",
	}, 0)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if id != 1 {
		t.Fatalf("id = %d, want 1", id)
	}

	got, err := c.GetIssue(context.Background(), id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Title != "via client" || got.Status != store.StatusOpen {
		t.Fatalf("GetIssue = %+v", got)
	}
	if got.Author != nil {
		t.Fatalf("env-token author should be anonymous, got %+v", got.Author)
	}

	list, _, err := c.ListIssues(context.Background(), store.ListFilter{Status: "all"})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListIssues len = %d", len(list))
	}

	if err := c.SetIssueStatus(context.Background(), id, store.StatusClosed); err != nil {
		t.Fatalf("SetIssueStatus: %v", err)
	}
	got, _ = c.GetIssue(context.Background(), id)
	if got.Status != store.StatusClosed {
		t.Fatalf("status = %q", got.Status)
	}

	cid, err := c.CreateComment(context.Background(), id, "a comment", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if cid != 1 {
		t.Fatalf("comment id = %d", cid)
	}
	cs, err := c.ListComments(context.Background(), id)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(cs) != 1 || cs[0].Body != "a comment" {
		t.Fatalf("ListComments = %+v", cs)
	}

	if err := c.UpdateIssue(context.Background(), id, store.IssueInput{Title: "renamed"}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	got, _ = c.GetIssue(context.Background(), id)
	if got.Title != "renamed" {
		t.Fatalf("title = %q", got.Title)
	}

	if err := c.DeleteIssue(context.Background(), id); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	if _, err := c.GetIssue(context.Background(), id); err == nil {
		t.Fatal("GetIssue after delete: want error")
	}
}

func TestClientPerUserToken(t *testing.T) {
	c, st := newFixture(t, "")
	uid, err := st.CreateUser(context.Background(), "dave", "hashed")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateAPIToken(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	c.token = tok

	id, err := c.CreateIssue(context.Background(), store.IssueInput{Title: "mine"}, 0)
	if err != nil {
		t.Fatalf("CreateIssue with user token: %v", err)
	}
	got, err := c.GetIssue(context.Background(), id)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Author == nil || got.Author.Name != "dave" {
		t.Fatalf("author = %+v, want dave", got.Author)
	}
}

func TestClientErrors(t *testing.T) {
	c, _ := newFixture(t, "right-token")
	c.token = "wrong-token"

	_, _, err := c.ListIssues(context.Background(), store.ListFilter{})
	ae, ok := err.(*Error)
	if !ok || ae.Status != 401 {
		t.Fatalf("want *Error 401, got %v", err)
	}

	c.token = "right-token"
	_, err = c.GetIssue(context.Background(), 999)
	ae, ok = err.(*Error)
	if !ok || ae.Status != 404 {
		t.Fatalf("want *Error 404, got %v", err)
	}
}

func TestClientLabels(t *testing.T) {
	c, st := newFixture(t, "env-tok")
	if err := st.SeedDemoData(context.Background()); err != nil {
		t.Fatal(err)
	}
	labels, err := c.ListLabels(context.Background(), store.KindLabelIssue)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) == 0 || labels[0].Name != "Bug" { // sorted by name
		t.Fatalf("ListLabels = %+v", labels)
	}
}

// Regression: Devin Review findings — unknown label filter and label
// clearing through PATCH.
func TestClientUnknownLabelFilter(t *testing.T) {
	c, st := newFixture(t, "env-tok")
	st.CreateIssue(context.Background(), 1, store.IssueInput{Title: "x"}, 0)

	got, _, err := c.ListIssues(context.Background(),
		store.ListFilter{Status: "all", LabelID: 9999})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unknown LabelID: want empty, got %d issues", len(got))
	}
}

func TestClientUpdateClearsLabels(t *testing.T) {
	c, st := newFixture(t, "env-tok")
	st.SeedDemoData(context.Background())
	id, err := st.CreateIssue(context.Background(), 1,
		store.IssueInput{Title: "labeled", LabelIDs: []int64{1}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateIssue(context.Background(), id,
		store.IssueInput{Title: "labeled"}); err != nil { // nil LabelIDs = clear
		t.Fatalf("UpdateIssue: %v", err)
	}
	got, err := c.GetIssue(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Labels) != 0 {
		t.Fatalf("labels after clear: %+v", got.Labels)
	}
}

// PerPage above the API cap must still honor Page semantics.
func TestClientPaginationAboveCap(t *testing.T) {
	c, st := newFixture(t, "env-tok")
	for i := 0; i < 250; i++ {
		if _, err := st.CreateIssue(context.Background(), 1,
			store.IssueInput{Title: "bulk"}, 0); err != nil {
			t.Fatal(err)
		}
	}
	// Sort oldest so ids match positions 1..250.
	got, _, err := c.ListIssues(context.Background(), store.ListFilter{
		Status: "all", Sort: "oldest", PerPage: 150, Page: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 100 || got[0].ID != 151 || got[99].ID != 250 {
		var first, last int64
		if len(got) > 0 {
			first, last = got[0].ID, got[len(got)-1].ID
		}
		t.Fatalf("want issues 151..250 (100 items), got len=%d [%d..%d]", len(got), first, last)
	}
}

// Regression: https→http redirects must be refused — Go would forward
// the Authorization header on a same-host redirect across the downgrade.
func TestHTTPClientRefusesDowngrade(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("request reached the plaintext redirect target")
		}))
	defer plain.Close()
	tls := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, plain.URL+"/x", http.StatusFound)
		}))
	defer tls.Close()

	req, err := http.NewRequestWithContext(context.Background(),
		"GET", tls.URL+"/y", nil)
	if err != nil {
		t.Fatal(err)
	}
	// TLS transport from the test server, redirect policy from HTTPClient.
	c := &http.Client{
		Transport:     tls.Client().Transport,
		CheckRedirect: HTTPClient.CheckRedirect,
	}
	resp, err := c.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("downgrade redirect was followed")
	}
	if !strings.Contains(err.Error(), "refusing https") {
		t.Fatalf("unexpected error: %v", err)
	}
}
