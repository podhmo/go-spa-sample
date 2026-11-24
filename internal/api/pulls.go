// Pull requests (docs/sketch/ja/pull-requests.md §7): the machine-facing
// surface over the same store+gitrepo pair the HTML /pulls pages use.
// These routes mount only when api.New got a *gitrepo.Repo — repo-less
// deployments answer the JSON 404 via the /api/v1 catch-all.
package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/export"
	"github.com/podhmo/go-spa-sample/internal/gitrepo"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// PullRequest is the wire shape of a store.PullRequest.
type PullRequest struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	Status      string  `json:"status"`
	Base        string  `json:"base"`
	Head        string  `json:"head"`
	Author      *User   `json:"author"`
	HeadSHA     string  `json:"head_sha,omitempty"`
	MergeSHA    string  `json:"merge_sha,omitempty"`
	MergeMethod string  `json:"merge_method,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	ClosedAt    *string `json:"closed_at,omitempty"`
}

func pullRequestDTO(p *store.PullRequest) PullRequest {
	out := PullRequest{
		ID: p.ID, Title: p.Title, Body: p.Body, Status: p.Status,
		Base: p.BaseRef, Head: p.HeadRef, HeadSHA: p.HeadSHA,
		Author:    userDTO(p.Author),
		CreatedAt: fmtTime(p.CreatedAt), UpdatedAt: fmtTime(p.UpdatedAt),
	}
	if p.Status == store.PRStatusMerged {
		out.MergeSHA, out.MergeMethod = p.MergeSHA, p.MergeMethod
	}
	if p.ClosedAt != nil {
		s := fmtTime(*p.ClosedAt)
		out.ClosedAt = &s
	}
	return out
}

// PRComment is the wire shape of a store.PRComment; the anchor fields are
// empty for plain conversation comments.
type PRComment struct {
	ID          int64  `json:"id"`
	Body        string `json:"body"`
	Path        string `json:"path,omitempty"`
	Line        int    `json:"line,omitempty"`
	Side        string `json:"side,omitempty"`
	AnchorSHA   string `json:"anchor_sha,omitempty"`
	DiffSnippet string `json:"diff_snippet,omitempty"`
	Author      *User  `json:"author"`
	CreatedAt   string `json:"created_at"`
	Moderation
}

func prCommentDTO(c *store.PRComment) PRComment {
	return PRComment{
		ID: c.ID, Body: c.Body,
		Path: c.Path, Line: c.Line, Side: c.Side,
		AnchorSHA: c.AnchorSHA, DiffSnippet: c.DiffSnippet,
		Author: userDTO(c.Author), CreatedAt: fmtTime(c.CreatedAt),
		Moderation: moderationDTO(c.Moderation),
	}
}

// PRCommit is one head-side commit in GET /pulls/{id}.
type PRCommit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// requirePull loads the {id} PR or answers the JSON error.
func (h *handler) requirePull(w http.ResponseWriter, r *http.Request) (*store.PullRequest, bool) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	pr, err := h.store.GetPullRequest(r.Context(), id)
	if err != nil {
		h.prStoreError(w, err)
		return nil, false
	}
	return pr, true
}

// requirePRComment loads the {cid} comment of pr or answers the error.
func (h *handler) requirePRComment(w http.ResponseWriter, r *http.Request, prID int64) (*store.PRComment, bool) {
	cid, ok := h.pathID(w, r, "cid")
	if !ok {
		return nil, false
	}
	comments, err := h.store.ListPRComments(r.Context(), prID)
	if err != nil {
		h.prStoreError(w, err)
		return nil, false
	}
	for _, c := range comments {
		if c.ID == cid {
			return c, true
		}
	}
	writeError(w, http.StatusNotFound, "Not found.", nil)
	return nil, false
}

// prStoreError maps store sentinel errors to HTTP statuses — the PR
// companions of storeError (kept separate so existing tests' error
// shapes stay untouched).
func (h *handler) prStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Not found.", nil)
	case errors.Is(err, store.ErrDuplicatePR):
		writeError(w, http.StatusUnprocessableEntity, "An open pull request already exists for this base/head pair.", nil)
	case errors.Is(err, store.ErrSameRef):
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"head": "Base and head must differ."})
	case errors.Is(err, store.ErrInvalidPRTransition):
		writeError(w, http.StatusConflict, "The pull request's status does not allow this change.", nil)
	case errors.Is(err, store.ErrInvalidAnchor):
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"path": "Invalid review-comment anchor."})
	default:
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
	}
}

// gitError maps gitrepo sentinel errors; the bool reports handled.
func (h *handler) gitError(w http.ResponseWriter, err error) bool {
	var ce *gitrepo.ConflictError
	var de *gitrepo.DirtyWorktreeError
	switch {
	case errors.Is(err, gitrepo.ErrBadRef):
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"head": "Base or head ref could not be resolved."})
	case errors.As(err, &ce):
		writeError(w, http.StatusConflict, "The branches conflict: "+strings.Join(ce.Files, ", "), nil)
	case errors.As(err, &de):
		writeError(w, http.StatusConflict, "The base branch's worktree is dirty.", nil)
	case errors.Is(err, gitrepo.ErrConcurrentUpdate):
		writeError(w, http.StatusConflict, "The base branch moved while merging — try again.", nil)
	default:
		return false
	}
	return true
}

// diffRange names the revisions the PR's code view covers: open PRs are
// live merge-base→head (GitHub's three-dot semantics), closed/merged PRs
// the recorded shas so the view survives branch deletion.
func (h *handler) diffRange(r *http.Request, pr *store.PullRequest) (from, to string) {
	if pr.Open() {
		from, to = pr.BaseRef, pr.HeadRef
		if mb, err := h.repo.MergeBase(r.Context(), pr.BaseRef, pr.HeadRef); err == nil && mb != "" {
			from = mb
		}
		return from, to
	}
	from, to = pr.MergeBaseSHA, pr.HeadSHA
	if from == "" {
		from = pr.BaseRef
	}
	if to == "" {
		to = gitrepo.PinRef(pr.ID)
	}
	return from, to
}

// GET /api/v1/pulls?status=&q=&sort=&page=&per_page=
func (h *handler) listPulls(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.PRListFilter{
		Status:  q.Get("status"),
		Query:   strings.TrimSpace(q.Get("q")),
		Sort:    q.Get("sort"),
		Page:    queryInt(r, "page", 1),
		PerPage: min(queryInt(r, "per_page", defaultPerPage), maxPerPage),
	}
	pulls, total, err := h.store.ListPullRequests(r.Context(), f)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	out := []PullRequest{}
	for _, p := range pulls {
		out = append(out, pullRequestDTO(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pulls": out, "total": total, "page": f.Page, "per_page": f.PerPage,
	})
}

type pullBody struct {
	Base  *string `json:"base"`
	Head  *string `json:"head"`
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

// POST /api/v1/pulls {"base","head","title","body"} → 201
func (h *handler) createPull(w http.ResponseWriter, r *http.Request) {
	var in pullBody
	if !decodeBody(w, r, &in) {
		return
	}
	ctx := r.Context()
	input := store.PRInput{
		Title: strings.TrimSpace(deref(in.Title)),
		Body:  strings.TrimSpace(deref(in.Body)),
	}
	base, head := deref(in.Base), deref(in.Head)
	var normErr error
	if input.BaseRef, normErr = h.repo.NormalizeRef(ctx, base); normErr != nil {
		input.BaseRef = base
	}
	if input.HeadRef, normErr = h.repo.NormalizeRef(ctx, head); normErr != nil {
		input.HeadRef = head
	}
	fe := store.ValidatePullRequestInput(input)
	if normErr != nil && !errors.Is(normErr, gitrepo.ErrBadRef) {
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		return
	}
	if normErr != nil {
		if fe == nil {
			fe = store.FieldErrors{}
		}
		fe["refs"] = store.FieldError{Code: "refs_invalid", Message: "Base and head must name existing refs or commits."}
	}
	if fe.Any() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	var shaErr error
	if input.BaseSHA, shaErr = h.repo.RevParse(ctx, input.BaseRef); shaErr != nil {
		if !h.gitError(w, shaErr) {
			writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		}
		return
	}
	if input.HeadSHA, shaErr = h.repo.RevParse(ctx, input.HeadRef); shaErr != nil {
		if !h.gitError(w, shaErr) {
			writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		}
		return
	}
	headSHA := input.HeadSHA
	id, err := h.store.CreatePullRequest(ctx, input, actorID(r), func(prID int64) error {
		return h.repo.PinHead(ctx, prID, headSHA)
	})
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	pr, err := h.store.GetPullRequest(ctx, id)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"pull_request": pullRequestDTO(pr)})
}

// GET /api/v1/pulls/{id} — one call returns the PR with its comments,
// the head-side commits, and the live mergeability verdict (open only).
func (h *handler) getPull(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	comments, err := h.store.ListPRComments(ctx, pr.ID)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	out := []PRComment{}
	for _, c := range comments {
		out = append(out, prCommentDTO(c))
	}
	resp := map[string]any{
		"pull_request": pullRequestDTO(pr),
		"comments":     out,
		"commits":      []PRCommit{},
	}
	if pr.Open() {
		if cs, err := h.repo.Log(ctx, pr.BaseRef, pr.HeadRef); err == nil {
			commits := make([]PRCommit, 0, len(cs))
			for _, c := range cs {
				commits = append(commits, PRCommit{
					SHA: c.SHA, Subject: c.Subject, Author: c.Author, Date: fmtTime(c.Date),
				})
			}
			resp["commits"] = commits
		}
		// mergeable: "clean" | "conflicting" | "unknown"; nil when the
		// probe could not run at all (e.g. the head ref is gone).
		if check, err := h.repo.CheckMerge(ctx, pr.BaseRef, pr.HeadRef); err == nil && check != nil {
			mergeable := map[string]any{"status": "unknown"}
			switch check.Status {
			case gitrepo.MergeClean:
				mergeable["status"] = "clean"
			case gitrepo.MergeConflicting:
				mergeable["status"] = "conflicting"
				mergeable["files"] = check.Files
			}
			resp["mergeable"] = mergeable
		}
	} else if cs, err := h.repo.Log(ctx, pr.MergeBaseSHA, pr.HeadSHA); err == nil {
		commits := make([]PRCommit, 0, len(cs))
		for _, c := range cs {
			commits = append(commits, PRCommit{
				SHA: c.SHA, Subject: c.Subject, Author: c.Author, Date: fmtTime(c.Date),
			})
		}
		resp["commits"] = commits
	}
	writeJSON(w, http.StatusOK, resp)
}

// PATCH /api/v1/pulls/{id} — title/body only; the ref pair is immutable.
func (h *handler) updatePull(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	var in pullBody
	if !decodeBody(w, r, &in) {
		return
	}
	input := store.PRInput{
		BaseRef: pr.BaseRef, HeadRef: pr.HeadRef,
		Title: pr.Title, Body: pr.Body,
	}
	if in.Title != nil {
		input.Title = strings.TrimSpace(*in.Title)
	}
	if in.Body != nil {
		input.Body = strings.TrimSpace(*in.Body)
	}
	if fe := store.ValidatePullRequestInput(input); fe.Any() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	if err := h.store.UpdatePullRequest(r.Context(), pr.ID, input); err != nil {
		h.prStoreError(w, err)
		return
	}
	pr, err := h.store.GetPullRequest(r.Context(), pr.ID)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pull_request": pullRequestDTO(pr)})
}

// DELETE /api/v1/pulls/{id} → 204 — removes the row (comments cascade)
// and the gc-protecting pin ref.
func (h *handler) deletePull(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	if err := h.store.DeletePullRequest(r.Context(), pr.ID); err != nil {
		h.prStoreError(w, err)
		return
	}
	// Best effort: the row is gone either way; a stuck pin only costs a
	// stray ref.
	_ = h.repo.Unpin(r.Context(), pr.ID)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/pulls/{id}/comments
func (h *handler) listPRComments(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	comments, err := h.store.ListPRComments(r.Context(), pr.ID)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	out := []PRComment{}
	for _, c := range comments {
		out = append(out, prCommentDTO(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

type prCommentBody struct {
	Body *string `json:"body"`
	Path *string `json:"path"`
	Line *int    `json:"line"`
	Side *string `json:"side"`
}

// POST /api/v1/pulls/{id}/comments {"body","path"?,"line"?,"side"?} → 201
func (h *handler) createPRComment(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	var in prCommentBody
	if !decodeBody(w, r, &in) {
		return
	}
	ctx := r.Context()
	input := store.PRCommentInput{Body: strings.TrimSpace(deref(in.Body))}
	if fe := store.ValidateCommentBody(input.Body); fe.Code != "" {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"body": fe.Message})
		return
	}
	if in.Path != nil && *in.Path != "" {
		line := 0
		if in.Line != nil {
			line = *in.Line
		}
		side := deref(in.Side)
		if err := gitrepo.ValidatePath(*in.Path); err != nil || line < 1 ||
			(side != "" && side != "right" && side != "left") {
			writeError(w, http.StatusUnprocessableEntity, "Validation failed.",
				map[string]string{"path": "Invalid review-comment anchor."})
			return
		}
		if side == "" {
			side = "right"
		}
		// A review comment must anchor to a file the PR actually touches:
		// accepting any repo path would let a token holder read unrelated
		// files via the returned diff_snippet.
		from, to := h.diffRange(r, pr)
		changes, cherr := h.repo.ChangedFiles(ctx, from, to)
		inDiff := false
		if cherr == nil {
			for _, c := range changes {
				if c.NewPath == *in.Path || c.OldPath == *in.Path {
					inDiff = true
					break
				}
			}
		}
		if cherr != nil || !inDiff {
			writeError(w, http.StatusUnprocessableEntity, "Validation failed.",
				map[string]string{"path": "Path is not part of the pull request diff."})
			return
		}
		input.Path, input.Line, input.Side = *in.Path, line, side
		// Anchor + snippet rules mirror the HTML handler: the head tip
		// while open (recorded sha once closed), the merge-base blob for
		// left-side comments.
		anchorSHA := pr.HeadSHA
		if pr.Open() {
			if sha, err := h.repo.RevParse(ctx, pr.HeadRef); err == nil {
				anchorSHA = sha
			}
		}
		input.AnchorSHA = anchorSHA
		snippetSHA := anchorSHA
		if side == "left" {
			snippetSHA = pr.MergeBaseSHA
			if pr.Open() {
				if mb, err := h.repo.MergeBase(ctx, pr.BaseRef, pr.HeadRef); err == nil {
					snippetSHA = mb
				}
			}
		}
		if snippetSHA != "" {
			if snippet, _, err := h.repo.Snippet(ctx, snippetSHA, input.Path, line, 2); err == nil {
				input.DiffSnippet = snippet
			}
		}
	}
	id, err := h.store.CreatePRComment(ctx, pr.ID, input, actorID(r))
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	c, ok := h.findPRComment(w, r, pr.ID, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"comment": prCommentDTO(c)})
}

func (h *handler) findPRComment(w http.ResponseWriter, r *http.Request, prID, commentID int64) (*store.PRComment, bool) {
	comments, err := h.store.ListPRComments(r.Context(), prID)
	if err != nil {
		h.prStoreError(w, err)
		return nil, false
	}
	for _, c := range comments {
		if c.ID == commentID {
			return c, true
		}
	}
	writeError(w, http.StatusNotFound, "Not found.", nil)
	return nil, false
}

// PATCH /api/v1/pulls/{id}/comments/{cid} {"body"} → 200
func (h *handler) updatePRComment(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	c, ok := h.requirePRComment(w, r, pr.ID)
	if !ok {
		return
	}
	var in prCommentBody
	if !decodeBody(w, r, &in) {
		return
	}
	body := c.Body
	if in.Body != nil {
		body = strings.TrimSpace(*in.Body)
	}
	if fe := store.ValidateCommentBody(body); fe.Code != "" {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"body": fe.Message})
		return
	}
	if err := h.store.UpdatePRComment(r.Context(), pr.ID, c.ID, body); err != nil {
		h.prStoreError(w, err)
		return
	}
	c, ok = h.findPRComment(w, r, pr.ID, c.ID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comment": prCommentDTO(c)})
}

// DELETE /api/v1/pulls/{id}/comments/{cid} → 204
func (h *handler) deletePRComment(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	c, ok := h.requirePRComment(w, r, pr.ID)
	if !ok {
		return
	}
	if err := h.store.DeletePRComment(r.Context(), pr.ID, c.ID); err != nil {
		h.prStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/pulls/{id}/comments/{cid}/hide {"reason"} → 200
func (h *handler) hidePRComment(w http.ResponseWriter, r *http.Request) {
	h.setPRCommentHidden(w, r, true)
}

// POST /api/v1/pulls/{id}/comments/{cid}/unhide → 200
func (h *handler) unhidePRComment(w http.ResponseWriter, r *http.Request) {
	h.setPRCommentHidden(w, r, false)
}

func (h *handler) setPRCommentHidden(w http.ResponseWriter, r *http.Request, hide bool) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	c, ok := h.requirePRComment(w, r, pr.ID)
	if !ok {
		return
	}
	reason := ""
	if hide {
		var ok bool
		if reason, ok = decodeHideBody(w, r); !ok {
			return
		}
	}
	if err := h.store.SetPRCommentHidden(r.Context(), pr.ID, c.ID, reason); err != nil {
		h.prStoreError(w, err)
		return
	}
	c, ok = h.findPRComment(w, r, pr.ID, c.ID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comment": prCommentDTO(c)})
}

type prStatusBody struct {
	Status string `json:"status"`
}

// POST /api/v1/pulls/{id}/status {"status":"closed"|"open"} — the same
// pin + recorded-sha rules as the HTML handler.
func (h *handler) setPullStatus(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	var in prStatusBody
	if !decodeBody(w, r, &in) {
		return
	}
	ctx := r.Context()
	switch in.Status {
	case "closed":
		if !pr.Open() {
			writeError(w, http.StatusConflict, "Only an open pull request can be closed.", nil)
			return
		}
		headSHA, err := h.repo.RevParse(ctx, pr.HeadRef)
		if err != nil {
			headSHA = "" // head may be deleted already — close anyway
		}
		if headSHA != "" {
			if err := h.repo.UpdatePin(ctx, pr.ID, headSHA); err != nil {
				writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
				return
			}
		}
		mergeBase, _ := h.repo.MergeBase(ctx, pr.BaseRef, pr.HeadRef)
		if err := h.store.ClosePullRequest(ctx, pr.ID, headSHA, mergeBase); err != nil {
			h.prStoreError(w, err)
			return
		}
	case "open":
		if err := h.store.ReopenPullRequest(ctx, pr.ID); err != nil {
			h.prStoreError(w, err)
			return
		}
	default:
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.",
			map[string]string{"status": "Unknown status action."})
		return
	}
	pr, err := h.store.GetPullRequest(ctx, pr.ID)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pull_request": pullRequestDTO(pr)})
}

type prMergeBody struct {
	Method  string  `json:"method"`
	Message *string `json:"message"`
}

// POST /api/v1/pulls/{id}/merge {"method":"merge|squash|rebase",
// "message"?} — git-side merge first (pre-flight checks live in
// gitrepo.Merge), then the DB records the result.
func (h *handler) mergePull(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	var in prMergeBody
	if !decodeBody(w, r, &in) {
		return
	}
	ctx := r.Context()
	if !pr.Open() {
		writeError(w, http.StatusConflict, "Only an open pull request can be merged.", nil)
		return
	}
	method := gitrepo.MergeMethod(in.Method)
	if in.Method == "" {
		method = gitrepo.MethodMerge
	}
	if !method.Valid() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.",
			map[string]string{"method": "Unknown merge method."})
		return
	}
	msg := deref(in.Message)
	if msg == "" && method == gitrepo.MethodSquash {
		msg = fmt.Sprintf("%s (#%d)", pr.Title, pr.ID)
	}
	res, err := h.repo.Merge(ctx, pr.BaseRef, pr.HeadRef, method, msg)
	if err != nil {
		if !h.gitError(w, err) {
			writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		}
		return
	}
	// Pin the resolved head so the recorded merge survives head-branch
	// deletion; the merge is already applied, so a failure degrades to a
	// stray unprotected commit rather than a false "merge failed".
	_ = h.repo.UpdatePin(ctx, pr.ID, res.HeadSHA)
	if err := h.store.MarkMerged(ctx, pr.ID, store.PRMergeInfo{
		MergeSHA:     res.MergeSHA,
		MergeMethod:  string(res.Method),
		HeadSHA:      res.HeadSHA,
		MergeBaseSHA: res.MergeBaseSHA,
	}); err != nil {
		h.prStoreError(w, err)
		return
	}
	pr, err = h.store.GetPullRequest(ctx, pr.ID)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pull_request": pullRequestDTO(pr)})
}

// GET /api/v1/pulls/{id}/diff → text/plain unified diff (merge-base→head
// while open, recorded shas once closed/merged).
func (h *handler) pullDiff(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	from, to := h.diffRange(r, pr)
	diff, _, err := h.repo.Diff(r.Context(), from, to)
	if err != nil {
		if !h.gitError(w, err) {
			writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		}
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, diff)
}

// GET /api/v1/pulls/{id}/markdown → text/markdown — remote `zatu pr view
// --markdown` calls this; rendered by internal/export.
func (h *handler) pullMarkdown(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.requirePull(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	comments, err := h.store.ListPRComments(ctx, pr.ID)
	if err != nil {
		h.prStoreError(w, err)
		return
	}
	var commits []export.PRCommit
	from, to := pr.BaseRef, pr.HeadRef // commit list keeps base..head
	if !pr.Open() {
		from, to = pr.MergeBaseSHA, pr.HeadSHA
	}
	if from != "" && to != "" {
		if cs, err := h.repo.Log(ctx, from, to); err == nil {
			for _, c := range cs {
				commits = append(commits, export.PRCommit{
					SHA: c.SHA, Subject: c.Subject, Author: c.Author, Date: c.Date,
				})
			}
		}
	}
	writeMarkdown(w, fmt.Sprintf("pull-%d.md", pr.ID),
		export.PullRequestMarkdown(export.PRMarkdownData{PR: pr, Commits: commits, Comments: comments}))
}
