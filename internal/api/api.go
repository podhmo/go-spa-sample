// Package api implements the JSON Web API under /api/v1
// (docs/sketch/ja/cli-and-web-api.md, Phase 2). It is a thin machine-facing
// surface over the same store the HTML app uses: DTOs are wire types kept
// separate from the store's domain types, and every request must
// authenticate (fail-closed 401s). Three credential kinds are accepted:
// the /auth/* cookie session (browser login), a single shared env token
// (writes stay anonymous), and per-user api_tokens rows (writes carry
// the user's author_id).
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/podhmo/go-spa-sample/internal/gitrepo"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// Store is the consumer-side persistence contract the API needs.
// *store.Store satisfies it automatically; tests may substitute a fake.
type Store interface {
	ListIssues(ctx context.Context, wsID int64, f store.ListFilter) ([]*store.Issue, int, error)
	GetIssue(ctx context.Context, id int64) (*store.Issue, error)
	CreateIssue(ctx context.Context, wsID int64, in store.IssueInput, authorID int64) (int64, error)
	UpdateIssue(ctx context.Context, id int64, in store.IssueInput) error
	SetIssueStatus(ctx context.Context, id int64, status string) error
	DeleteIssue(ctx context.Context, id int64) error
	ListComments(ctx context.Context, issueID int64) ([]*store.Comment, error)
	CreateComment(ctx context.Context, issueID int64, body string, authorID int64) (int64, error)
	UpdateComment(ctx context.Context, issueID, commentID int64, body string) error
	DeleteComment(ctx context.Context, issueID, commentID int64) error
	SetCommentHidden(ctx context.Context, issueID, commentID int64, reason string) error
	ListLabels(ctx context.Context, wsID int64, kind string) ([]*store.Label, error)
	ListArchivedLabels(ctx context.Context, wsID int64, kind string) ([]*store.Label, error)
	GetLabel(ctx context.Context, id int64) (*store.Label, error)
	CreateLabel(ctx context.Context, wsID int64, kind string, in store.LabelInput) (int64, error)
	UpdateLabel(ctx context.Context, id int64, in store.LabelInput) error
	ArchiveLabel(ctx context.Context, id int64, archived bool) error
	ListDiscussions(ctx context.Context, wsID int64, f store.DiscussionListFilter) ([]*store.Discussion, int, error)
	GetDiscussion(ctx context.Context, id int64) (*store.Discussion, error)
	CreateDiscussion(ctx context.Context, wsID int64, in store.DiscussionInput, authorID int64) (int64, error)
	UpdateDiscussion(ctx context.Context, id int64, in store.DiscussionInput) error
	DeleteDiscussion(ctx context.Context, id int64) error
	ListDiscussionThreads(ctx context.Context, discussionID int64) ([]*store.DiscussionThread, error)
	CreateDiscussionThread(ctx context.Context, discussionID int64, body string, authorID int64) (int64, error)
	CreateDiscussionComment(ctx context.Context, discussionID, threadID int64, body string, authorID int64) (int64, error)
	SetThreadAnswer(ctx context.Context, discussionID, threadID int64, answer bool) error
	UpdateDiscussionThread(ctx context.Context, discussionID, threadID int64, body string) error
	DeleteDiscussionThread(ctx context.Context, discussionID, threadID int64) error
	SetDiscussionThreadHidden(ctx context.Context, discussionID, threadID int64, reason string) error
	UpdateDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64, body string) error
	DeleteDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64) error
	SetDiscussionCommentHidden(ctx context.Context, discussionID, threadID, commentID int64, reason string) error
	ListDiscussionCategories(ctx context.Context) ([]*store.DiscussionCategory, error)
	ListWorkspaces(ctx context.Context) ([]*store.Workspace, error)
	WorkspaceBySlug(ctx context.Context, slug string) (*store.Workspace, error)
	WorkspaceByID(ctx context.Context, id int64) (*store.Workspace, error)
	CreateWorkspace(ctx context.Context, in store.WorkspaceInput) (int64, error)
	DeleteWorkspace(ctx context.Context, slug string) error
	APITokenUser(ctx context.Context, token string) (*store.User, error)

	// Accounts & cookie sessions (internal/api/auth.go).
	CreateUser(ctx context.Context, name, passwordHash string) (int64, error)
	UserByName(ctx context.Context, name string) (*store.User, error)
	AuthenticateUser(ctx context.Context, name, password string) (*store.User, error)
	UserByID(ctx context.Context, id int64) (*store.User, error)
	CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, error)
	SessionUser(ctx context.Context, token string, ttl time.Duration) (*store.User, error)
	DeleteSession(ctx context.Context, token string) error

	// Pull requests — workspace-independent (docs/sketch/ja/
	// pull-requests.md §6). These mount only with a *gitrepo.Repo.
	ListPullRequests(ctx context.Context, f store.PRListFilter) ([]*store.PullRequest, int, error)
	GetPullRequest(ctx context.Context, id int64) (*store.PullRequest, error)
	CreatePullRequest(ctx context.Context, in store.PRInput, authorID int64, pin func(prID int64) error) (int64, error)
	UpdatePullRequest(ctx context.Context, id int64, in store.PRInput) error
	DeletePullRequest(ctx context.Context, id int64) error
	ClosePullRequest(ctx context.Context, id int64, headSHA, mergeBaseSHA string) error
	ReopenPullRequest(ctx context.Context, id int64) error
	MarkMerged(ctx context.Context, id int64, mi store.PRMergeInfo) error
	ListPRComments(ctx context.Context, prID int64) ([]*store.PRComment, error)
	CreatePRComment(ctx context.Context, prID int64, in store.PRCommentInput, authorID int64) (int64, error)
	UpdatePRComment(ctx context.Context, prID, commentID int64, body string) error
	DeletePRComment(ctx context.Context, prID, commentID int64) error
	SetPRCommentHidden(ctx context.Context, prID, commentID int64, reason string) error
}

const (
	// maxBodyBytes caps JSON request bodies, matching the HTML form limit.
	maxBodyBytes = 1 << 20 // 1 MiB
	// defaultPerPage matches the HTML list's fixed page size.
	defaultPerPage = 20
	maxPerPage     = 100
)

// New returns the /api/v1 handler. /api/v1/auth/* answers without a
// credential — it is how one is obtained — and every other path needs a
// session cookie or "Authorization: Bearer <token>". envToken is the
// optional shared credential (constant-time compared; its requests are
// anonymous); any other token is resolved through the store's api_tokens
// table and its requests run as that user.
func New(st Store, envToken string, repo *gitrepo.Repo) http.Handler {
	h := &handler{store: st, repo: repo}
	mux := http.NewServeMux()

	// Routes are registered methodless per path and dispatched on
	// r.Method: method-scoped patterns would only 405 when no catch-all
	// matches, while a methodless catch-all would eat them into a 404.
	// Dispatching here gives both — known path + wrong method → 405,
	// unknown path → JSON 404 via the "/api/v1/" subtree below.
	// Workspace-scoped resources live under /w/{slug}; the workspace-less
	// /issues path is gone (a 404, not a redirect — API callers should be
	// updated, docs/sketch/ja/workspaces.md). Labels are workspace-scoped
	// too (shared master rows ride along). Discussions are workspace-
	// scoped like issues; pull requests stay workspace-independent and
	// mount separately (docs/sketch/ja/pull-requests.md §6).
	routes := map[string]map[string]http.HandlerFunc{
		"/api/v1/workspaces":        {"GET": h.listWorkspaces, "POST": h.createWorkspace},
		"/api/v1/workspaces/{slug}": {"GET": h.getWorkspace, "DELETE": h.deleteWorkspace},

		"/api/v1/w/{ws}/labels":      {"GET": h.scoped(h.listLabels), "POST": h.scoped(h.createLabel)},
		"/api/v1/w/{ws}/labels/{id}": {"PATCH": h.scoped(h.updateLabel), "DELETE": h.scoped(h.deleteLabel)},

		"/api/v1/w/{ws}/issues":               {"GET": h.scoped(h.listIssues), "POST": h.scoped(h.createIssue)},
		"/api/v1/w/{ws}/issues/{id}":          {"GET": h.scoped(h.getIssue), "PATCH": h.scoped(h.updateIssue), "DELETE": h.scoped(h.deleteIssue)},
		"/api/v1/w/{ws}/issues/{id}/status":   {"POST": h.scoped(h.setIssueStatus)},
		"/api/v1/w/{ws}/issues/{id}/comments": {"POST": h.scoped(h.createComment)},
		// Comment moderation (internal/api/comments.go): PATCH/DELETE on
		// the comment itself, hide/unhide as explicit action POSTs.
		"/api/v1/w/{ws}/issues/{id}/comments/{cid}":        {"PATCH": h.scoped(h.updateComment), "DELETE": h.scoped(h.deleteComment)},
		"/api/v1/w/{ws}/issues/{id}/comments/{cid}/hide":   {"POST": h.scoped(h.hideComment)},
		"/api/v1/w/{ws}/issues/{id}/comments/{cid}/unhide": {"POST": h.scoped(h.unhideComment)},
		"/api/v1/w/{ws}/issues/{id}/markdown":              {"GET": h.scoped(h.issueMarkdown)},

		// Discussions (internal/api/discussions.go): threads are the
		// top-level posts — only they can carry the answer mark — and
		// comments are flat replies inside one thread. Categories are
		// global seed data, so they stay workspace-less.
		"/api/v1/discussion-categories": {"GET": h.listDiscussionCategories},

		// Inside the gate: reports the credential's user (401 anonymous).
		"/api/v1/auth/me": {"GET": h.me},

		"/api/v1/w/{ws}/discussions":                                          {"GET": h.scoped(h.listDiscussions), "POST": h.scoped(h.createDiscussion)},
		"/api/v1/w/{ws}/discussions/{id}":                                     {"GET": h.scoped(h.getDiscussion), "PATCH": h.scoped(h.updateDiscussion), "DELETE": h.scoped(h.deleteDiscussion)},
		"/api/v1/w/{ws}/discussions/{id}/markdown":                            {"GET": h.scoped(h.discussionMarkdown)},
		"/api/v1/w/{ws}/discussions/{id}/threads":                             {"POST": h.scoped(h.createDiscussionThread)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}":                       {"PATCH": h.scoped(h.updateDiscussionThread), "DELETE": h.scoped(h.deleteDiscussionThread)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}/answer":                {"POST": h.scoped(h.setDiscussionThreadAnswer)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}/hide":                  {"POST": h.scoped(h.hideDiscussionThread)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}/unhide":                {"POST": h.scoped(h.unhideDiscussionThread)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}/comments":              {"POST": h.scoped(h.createDiscussionReply)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}/comments/{cid}":        {"PATCH": h.scoped(h.updateDiscussionReply), "DELETE": h.scoped(h.deleteDiscussionReply)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}/comments/{cid}/hide":   {"POST": h.scoped(h.hideDiscussionReply)},
		"/api/v1/w/{ws}/discussions/{id}/threads/{tid}/comments/{cid}/unhide": {"POST": h.scoped(h.unhideDiscussionReply)},
	}
	for path, methods := range routes {
		mux.HandleFunc(path, methodDispatch(methods))
	}

	// Pull requests mount only when the server has a git repo — on a
	// repo-less deployment /api/v1/pulls* falls to the JSON 404 below.
	if repo != nil {
		for path, methods := range map[string]map[string]http.HandlerFunc{
			"/api/v1/pulls/refs":                       {"GET": h.listPullRefs},
			"/api/v1/pulls":                            {"GET": h.listPulls, "POST": h.createPull},
			"/api/v1/pulls/{id}":                       {"GET": h.getPull, "PATCH": h.updatePull, "DELETE": h.deletePull},
			"/api/v1/pulls/{id}/comments":              {"GET": h.listPRComments, "POST": h.createPRComment},
			"/api/v1/pulls/{id}/comments/{cid}":        {"PATCH": h.updatePRComment, "DELETE": h.deletePRComment},
			"/api/v1/pulls/{id}/comments/{cid}/hide":   {"POST": h.hidePRComment},
			"/api/v1/pulls/{id}/comments/{cid}/unhide": {"POST": h.unhidePRComment},
			"/api/v1/pulls/{id}/status":                {"POST": h.setPullStatus},
			"/api/v1/pulls/{id}/merge":                 {"POST": h.mergePull},
			"/api/v1/pulls/{id}/diff":                  {"GET": h.pullDiff},
			"/api/v1/pulls/{id}/markdown":              {"GET": h.pullMarkdown},
		} {
			mux.HandleFunc(path, methodDispatch(methods))
		}
	}

	// Anything else under /api/v1 is a JSON 404 (HTML 404s would confuse
	// non-browser clients).
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "Not found.", nil)
	})

	// The auth endpoints sit on an outer mux outside the credential gate —
	// login must be reachable before the caller holds any credential.
	top := http.NewServeMux()
	for path, methods := range map[string]map[string]http.HandlerFunc{
		"/api/v1/auth/register": {"POST": requireJSON(h.register)},
		"/api/v1/auth/login":    {"POST": requireJSON(h.login)},
		"/api/v1/auth/logout":   {"POST": requireJSON(h.logout)},
	} {
		top.HandleFunc(path, methodDispatch(methods))
	}
	top.Handle("/api/v1/", authenticate(envToken, st, mux))
	return top
}

// requireJSON rejects bodies that did not opt in to application/json.
// A cross-origin browser fetch can only send a "simple" content type
// (form or text/plain) without a CORS preflight — and this API emits no
// CORS headers — so an application/json requirement is what stops a
// hostile site from driving login/register as the visitor (a form post
// or a text/plain fetch with a JSON body would otherwise reach
// decodeBody and work).
func requireJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" && !strings.HasPrefix(ct, "application/json;") {
			writeError(w, http.StatusUnsupportedMediaType,
				"Content-Type must be application/json.", nil)
			return
		}
		next(w, r)
	}
}

// methodDispatch runs the handler registered for r.Method, or answers 405
// with an Allow header when the path exists but not the method.
func methodDispatch(methods map[string]http.HandlerFunc) http.HandlerFunc {
	var names []string
	for m := range methods {
		names = append(names, m)
	}
	// HEAD is accepted wherever GET is (mapped below) — list it like the
	// std mux does so the Allow header tells the truth.
	if _, ok := methods[http.MethodGet]; ok {
		names = append(names, http.MethodHead)
	}
	slices.Sort(names)
	allow := strings.Join(names, ", ")
	return func(w http.ResponseWriter, r *http.Request) {
		// HEAD rides on GET, like the std mux's method-scoped patterns;
		// the server strips the body on the way out.
		m := r.Method
		if m == http.MethodHead {
			m = http.MethodGet
		}
		if fn, ok := methods[m]; ok {
			fn(w, r)
			return
		}
		w.Header().Set("Allow", allow)
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", nil)
	}
}

// authenticate rejects requests without a credential: either a
// "session" cookie (register/login issue it — the browser's identity) or
// a "Bearer <token>" Authorization header. The shared env token is
// compared in constant time and authenticates as anonymous (actor id 0);
// anything else is looked up in api_tokens, where the row's user becomes
// the request's actor. The header check is case-insensitive per RFC 9110
// field-name and auth-scheme rules. A valid session cookie wins over the
// header: in dev the proxy injects the shared token on every request,
// and the logged-in browser still wants its own identity.
func authenticate(envToken string, st Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
			u, err := st.SessionUser(r.Context(), c.Value, sessionTTL)
			switch {
			case err == nil:
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, u.ID)))
				return
			case errors.Is(err, store.ErrNotFound):
				// Unknown/expired session — fall through to the bearer check.
			default:
				writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
				return
			}
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || got == "" {
			writeError(w, http.StatusUnauthorized, "Unauthorized.", nil)
			return
		}
		var actorID int64
		if envToken != "" && subtle.ConstantTimeCompare([]byte(got), []byte(envToken)) == 1 {
			actorID = 0 // shared token: no user identity, writes stay anonymous
		} else {
			u, err := st.APITokenUser(r.Context(), got)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeError(w, http.StatusUnauthorized, "Unauthorized.", nil)
				} else {
					writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
				}
				return
			}
			actorID = u.ID
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actorID)))
	})
}

// actorKey is the context key under which bearerAuth stores the resolved
// user id (0 = anonymous).
type actorKey struct{}

// actorID returns the authenticated user id for the request, or 0 when the
// request used the shared env token (anonymous writes).
func actorID(r *http.Request) int64 {
	id, _ := r.Context().Value(actorKey{}).(int64)
	return id
}

type handler struct {
	store Store
	repo  *gitrepo.Repo // nil → /api/v1/pulls* unmounted
	// pullsMu serializes PR status transitions against merges: a close
	// racing an in-flight merge must not let the merge land unrecorded.
	pullsMu sync.Mutex
}

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func userDTO(u *store.User) *User {
	if u == nil {
		return nil
	}
	return &User{ID: u.ID, Name: u.Name}
}

func fmtTime(t time.Time) string { return t.Format(time.RFC3339) }

// writeJSON answers with a JSON body. All API responses use the same
// envelope style: resources under their plural name, errors under "error".
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeMarkdown answers a text/markdown document — the thread export from
// internal/export, the API's one non-JSON representation. Errors keep the
// JSON envelope; only 200s carry markdown.
func writeMarkdown(w http.ResponseWriter, filename, doc string) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	io.WriteString(w, doc)
}

// writeError emits {"error":{"message":..., "fields":{...}}} where fields
// is only present for per-field validation failures.
func writeError(w http.ResponseWriter, status int, message string, fields map[string]string) {
	e := map[string]any{"message": message}
	if len(fields) > 0 {
		e["fields"] = fields
	}
	writeJSON(w, status, map[string]any{"error": e})
}

// decodeBody reads a JSON object body; oversized or malformed bodies are a
// 400.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body.", nil)
		return false
	}
	// A valid first object must be the whole body — extra JSON or garbage
	// after it is rejected rather than silently dropped.
	if dec.More() {
		writeError(w, http.StatusBadRequest, "Invalid JSON body.", nil)
		return false
	}
	return true
}

// pathID parses a {id}-style path value; non-numeric or non-positive ids
// are a 404, matching the HTML handlers' treatment of parseID failures.
func (h *handler) pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "Not found.", nil)
		return 0, false
	}
	return id, true
}

// queryInt reads a positive int query parameter with a default.
func queryInt(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// scoped resolves the {ws} path segment into its workspace (a JSON 404 on
// a miss) and calls fn with it.
func (h *handler) scoped(fn func(http.ResponseWriter, *http.Request, *store.Workspace)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := h.store.WorkspaceBySlug(r.Context(), r.PathValue("ws"))
		if err != nil {
			h.storeError(w, err)
			return
		}
		fn(w, r, ws)
	}
}

// requireScope replies 404 when the resource belongs to another
// workspace — the API's slug-mismatch contract (no redirect like HTML).
// For GET/HEAD the real workspace's slug is returned two ways: in the
// error body as "workspace" and in the X-Workspace-Slug header (the
// header survives HEAD's stripped body). A client can rebase onto the
// canonical /w/{slug}/... URL itself — the in-band counterpart of the
// MPA's 302. Mutations get no hint — a rejected write must not invite a
// replay elsewhere.
func (h *handler) requireScope(w http.ResponseWriter, r *http.Request, ws *store.Workspace, realWSID int64) bool {
	if realWSID == ws.ID {
		return true
	}
	e := map[string]any{"message": "Not found."}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if real, err := h.store.WorkspaceByID(r.Context(), realWSID); err == nil {
			w.Header().Set("X-Workspace-Slug", real.Slug)
			e["workspace"] = real.Slug
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": e})
	return false
}

// storeError maps store sentinel errors to HTTP statuses.
func (h *handler) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Not found.", nil)
	case errors.Is(err, store.ErrDuplicateSlug):
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"slug": "This slug is already taken."})
	case errors.Is(err, store.ErrLastWorkspace):
		writeError(w, http.StatusUnprocessableEntity, "The last remaining workspace cannot be deleted.", nil)
	case errors.Is(err, store.ErrInvalidCategory):
		writeError(w, http.StatusUnprocessableEntity, "Unknown discussion category.", map[string]string{"category": "Unknown category."})
	case errors.Is(err, store.ErrInvalidThread):
		writeError(w, http.StatusUnprocessableEntity, "Replies can only be added to a thread of this discussion.", map[string]string{"thread_id": "Invalid reply target."})
	case errors.Is(err, store.ErrNotAnswerable):
		writeError(w, http.StatusUnprocessableEntity, "This thread cannot be marked as the answer.", nil)
	case errors.Is(err, store.ErrInvalidHideReason):
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", hideReasonField())
	default:
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
	}
}

// fields converts store.FieldErrors into the wire map (nil-safe).
func fields(fe store.FieldErrors) map[string]string {
	if len(fe) == 0 {
		return nil
	}
	m := make(map[string]string, len(fe))
	for k, v := range fe {
		m[k] = v.Message
	}
	return m
}
