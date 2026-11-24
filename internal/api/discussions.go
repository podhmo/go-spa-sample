// Discussion endpoints — the JSON counterpart of the MPA's /discussions
// pages (internal/web/discussions.go in go-classic-mpa-sample). A
// discussion holds threads (opening posts, answerable in qa categories);
// each thread holds flat comment replies. Only threads can carry the
// answer mark, so the wire keeps one level of nesting like the schema.
package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/export"
	"github.com/podhmo/go-spa-sample/internal/store"
)

type DiscussionCategory struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Format      string `json:"format"` // "open" | "qa"
}

type Discussion struct {
	ID        int64              `json:"id"`
	Title     string             `json:"title"`
	Body      string             `json:"body"`
	Category  DiscussionCategory `json:"category"`
	Labels    []Label            `json:"labels"`
	Author    *User              `json:"author"`
	Answered  bool               `json:"answered"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
}

type DiscussionThread struct {
	ID        int64             `json:"id"`
	IsAnswer  bool              `json:"is_answer"`
	Body      string            `json:"body"`
	Author    *User             `json:"author"`
	CreatedAt string            `json:"created_at"`
	Comments  []DiscussionReply `json:"comments"`
	Moderation
}

type DiscussionReply struct {
	ID        int64  `json:"id"`
	ThreadID  int64  `json:"thread_id"`
	Body      string `json:"body"`
	Author    *User  `json:"author"`
	CreatedAt string `json:"created_at"`
	Moderation
}

func categoryDTO(c store.DiscussionCategory) DiscussionCategory {
	return DiscussionCategory{
		ID: c.ID, Name: c.Name, Slug: c.Slug, Description: c.Description, Format: c.Format,
	}
}

func discussionDTO(d *store.Discussion) Discussion {
	out := Discussion{
		ID: d.ID, Title: d.Title, Body: d.Body, Category: categoryDTO(d.Category),
		Author: userDTO(d.Author), Answered: d.Answered,
		CreatedAt: fmtTime(d.CreatedAt), UpdatedAt: fmtTime(d.UpdatedAt),
		Labels: []Label{},
	}
	for _, l := range d.Labels {
		out.Labels = append(out.Labels, labelDTO(l))
	}
	return out
}

func threadDTO(t *store.DiscussionThread) DiscussionThread {
	out := DiscussionThread{
		ID: t.ID, IsAnswer: t.IsAnswer, Body: t.Body, Author: userDTO(t.Author),
		CreatedAt: fmtTime(t.CreatedAt), Moderation: moderationDTO(t.Moderation),
		Comments: []DiscussionReply{},
	}
	for _, c := range t.Comments {
		out.Comments = append(out.Comments, replyDTO(c))
	}
	return out
}

func replyDTO(c *store.DiscussionComment) DiscussionReply {
	return DiscussionReply{
		ID: c.ID, ThreadID: c.ThreadID, Body: c.Body, Author: userDTO(c.Author),
		CreatedAt: fmtTime(c.CreatedAt), Moderation: moderationDTO(c.Moderation),
	}
}

// GET /api/v1/discussion-categories → {"categories": [...]} — the seed
// categories are global master data, so this stays workspace-less like
// /api/v1/workspaces.
func (h *handler) listDiscussionCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.store.ListDiscussionCategories(r.Context())
	if err != nil {
		h.storeError(w, err)
		return
	}
	out := []DiscussionCategory{}
	for _, c := range cats {
		out = append(out, categoryDTO(*c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

// GET /api/v1/w/{ws}/discussions?category=&answered=&q=&sort=&page=&per_page=
// answered is "yes"/"no" like the HTML filter; anything else is ignored.
func (h *handler) listDiscussions(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	q := r.URL.Query()
	f := store.DiscussionListFilter{
		CategorySlug: q.Get("category"),
		Answered:     q.Get("answered"),
		Query:        strings.TrimSpace(q.Get("q")),
		Sort:         q.Get("sort"),
		Page:         queryInt(r, "page", 1),
		PerPage:      min(queryInt(r, "per_page", defaultPerPage), maxPerPage),
	}
	if f.Answered != "yes" && f.Answered != "no" {
		f.Answered = ""
	}
	if f.Sort != "oldest" {
		f.Sort = ""
	}
	discussions, total, err := h.store.ListDiscussions(r.Context(), ws.ID, f)
	if err != nil {
		h.storeError(w, err)
		return
	}
	out := []Discussion{}
	for _, d := range discussions {
		out = append(out, discussionDTO(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"discussions": out, "total": total, "page": f.Page, "per_page": f.PerPage,
	})
}

// requireDiscussion resolves /discussions/{id} and the workspace scope;
// any failure already wrote its response.
func (h *handler) requireDiscussion(w http.ResponseWriter, r *http.Request, ws *store.Workspace) (*store.Discussion, bool) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	d, err := h.store.GetDiscussion(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return nil, false
	}
	if !h.requireScope(w, r, ws, d.WorkspaceID) {
		return nil, false
	}
	return d, true
}

// findDiscussionThread re-reads one thread inside a discussion; nil when
// the list cannot be read or the id is gone (a reply id in place of a
// thread id also misses — the shapes never alias).
func (h *handler) findDiscussionThread(r *http.Request, discussionID, threadID int64) *store.DiscussionThread {
	threads, err := h.store.ListDiscussionThreads(r.Context(), discussionID)
	if err != nil {
		return nil
	}
	for _, t := range threads {
		if t.ID == threadID {
			return t
		}
	}
	return nil
}

// requireDiscussionThread resolves the discussion + scope + thread chain.
func (h *handler) requireDiscussionThread(w http.ResponseWriter, r *http.Request, ws *store.Workspace) (*store.Discussion, *store.DiscussionThread, bool) {
	d, ok := h.requireDiscussion(w, r, ws)
	if !ok {
		return nil, nil, false
	}
	tid, ok := h.pathID(w, r, "tid")
	if !ok {
		return nil, nil, false
	}
	t := h.findDiscussionThread(r, d.ID, tid)
	if t == nil {
		writeError(w, http.StatusNotFound, "Not found.", nil)
		return nil, nil, false
	}
	return d, t, true
}

// requireDiscussionReply resolves discussion + scope + thread + reply.
func (h *handler) requireDiscussionReply(w http.ResponseWriter, r *http.Request, ws *store.Workspace) (*store.DiscussionThread, *store.DiscussionComment, bool) {
	_, t, ok := h.requireDiscussionThread(w, r, ws)
	if !ok {
		return nil, nil, false
	}
	cid, ok := h.pathID(w, r, "cid")
	if !ok {
		return nil, nil, false
	}
	for _, c := range t.Comments {
		if c.ID == cid {
			return t, c, true
		}
	}
	writeError(w, http.StatusNotFound, "Not found.", nil)
	return nil, nil, false
}

// GET /api/v1/w/{ws}/discussions/{id} → {"discussion", "threads"} — one
// call returns the discussion plus its threaded comments, matching the
// HTML detail page's shape.
func (h *handler) getDiscussion(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	d, ok := h.requireDiscussion(w, r, ws)
	if !ok {
		return
	}
	threads, err := h.store.ListDiscussionThreads(r.Context(), d.ID)
	if err != nil {
		h.storeError(w, err)
		return
	}
	out := []DiscussionThread{}
	for _, t := range threads {
		out = append(out, threadDTO(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"discussion": discussionDTO(d), "threads": out})
}

// GET /api/v1/w/{ws}/discussions/{id}/markdown → text/markdown, the
// "Copy as markdown" export rendered by internal/export.
func (h *handler) discussionMarkdown(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	d, ok := h.requireDiscussion(w, r, ws)
	if !ok {
		return
	}
	threads, err := h.store.ListDiscussionThreads(r.Context(), d.ID)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeMarkdown(w, fmt.Sprintf("discussion-%d.md", d.ID), export.DiscussionMarkdown(d, threads, ws.Slug))
}

type discussionBody struct {
	Title      *string  `json:"title"`
	Body       *string  `json:"body"`
	CategoryID *int64   `json:"category_id"`
	Category   *string  `json:"category"` // slug, resolved via ListDiscussionCategories
	LabelIDs   []int64  `json:"label_ids"`
	Labels     []string `json:"labels"`
}

// resolveCategoryID turns the request's category (id or slug) into an id.
// category_id wins when both are present; an unknown slug is a 422 field
// error while a store lookup failure is an error.
func (h *handler) resolveCategoryID(r *http.Request, id *int64, slug *string) (int64, map[string]string, error) {
	if id != nil {
		return *id, nil, nil
	}
	if slug == nil || strings.TrimSpace(*slug) == "" {
		return 0, nil, nil
	}
	cats, err := h.store.ListDiscussionCategories(r.Context())
	if err != nil {
		return 0, nil, err
	}
	for _, c := range cats {
		if c.Slug == strings.TrimSpace(*slug) {
			return c.ID, nil, nil
		}
	}
	return 0, map[string]string{"category": "Unknown category: " + *slug}, nil
}

// POST /api/v1/w/{ws}/discussions {"title", "body", "category_id"|"category", "label_ids"|"labels"} → 201
func (h *handler) createDiscussion(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	var in discussionBody
	if !decodeBody(w, r, &in) {
		return
	}
	catID, ferr, err := h.resolveCategoryID(r, in.CategoryID, in.Category)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if ferr != nil {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", ferr)
		return
	}
	labelIDs, ferr, err := h.resolveLabelIDs(r, ws, store.KindLabelDiscussion, in.LabelIDs, in.Labels)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if ferr != nil {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", ferr)
		return
	}
	input := store.DiscussionInput{
		Title:      strings.TrimSpace(deref(in.Title)),
		Body:       strings.TrimSpace(deref(in.Body)),
		CategoryID: catID,
		LabelIDs:   labelIDs,
	}
	if fe := store.ValidateDiscussionInput(input); fe.Any() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	id, err := h.store.CreateDiscussion(r.Context(), ws.ID, input, actorID(r))
	if err != nil {
		h.storeError(w, err)
		return
	}
	d, err := h.store.GetDiscussion(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"discussion": discussionDTO(d)})
}

// PATCH /api/v1/w/{ws}/discussions/{id} — only fields present in the body
// are updated; absent fields keep their current values. An explicitly
// empty "labels"/"label_ids" clears the label set (nil means absent).
func (h *handler) updateDiscussion(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	var in discussionBody
	if !decodeBody(w, r, &in) {
		return
	}
	d, ok := h.requireDiscussion(w, r, ws)
	if !ok {
		return
	}
	input := store.DiscussionInput{Title: d.Title, Body: d.Body, CategoryID: d.Category.ID}
	for _, l := range d.Labels {
		input.LabelIDs = append(input.LabelIDs, l.ID)
	}
	if in.Title != nil {
		input.Title = strings.TrimSpace(*in.Title)
	}
	if in.Body != nil {
		input.Body = strings.TrimSpace(*in.Body)
	}
	if in.CategoryID != nil || in.Category != nil {
		catID, ferr, err := h.resolveCategoryID(r, in.CategoryID, in.Category)
		if err != nil {
			h.storeError(w, err)
			return
		}
		if ferr != nil {
			writeError(w, http.StatusUnprocessableEntity, "Validation failed.", ferr)
			return
		}
		input.CategoryID = catID
	}
	if in.LabelIDs != nil || in.Labels != nil {
		labelIDs, ferr, err := h.resolveLabelIDs(r, ws, store.KindLabelDiscussion, in.LabelIDs, in.Labels)
		if err != nil {
			h.storeError(w, err)
			return
		}
		if ferr != nil {
			writeError(w, http.StatusUnprocessableEntity, "Validation failed.", ferr)
			return
		}
		input.LabelIDs = labelIDs
	}
	if fe := store.ValidateDiscussionInput(input); fe.Any() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	if err := h.store.UpdateDiscussion(r.Context(), d.ID, input); err != nil {
		h.storeError(w, err)
		return
	}
	d, err := h.store.GetDiscussion(r.Context(), d.ID)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"discussion": discussionDTO(d)})
}

// DELETE /api/v1/w/{ws}/discussions/{id} → 204
func (h *handler) deleteDiscussion(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	d, ok := h.requireDiscussion(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.DeleteDiscussion(r.Context(), d.ID); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/w/{ws}/discussions/{id}/threads {"body"} → 201 {"thread"} —
// a new top-level comment thread (the MPA's comment form without
// thread_id).
func (h *handler) createDiscussionThread(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	body, ok := decodeEditBody(w, r)
	if !ok {
		return
	}
	d, ok := h.requireDiscussion(w, r, ws)
	if !ok {
		return
	}
	tid, err := h.store.CreateDiscussionThread(r.Context(), d.ID, body, actorID(r))
	if err != nil {
		h.storeError(w, err)
		return
	}
	if t := h.findDiscussionThread(r, d.ID, tid); t != nil {
		writeJSON(w, http.StatusCreated, map[string]any{"thread": threadDTO(t)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"thread": map[string]any{"id": tid}})
}

// PATCH /api/v1/w/{ws}/discussions/{id}/threads/{tid} {"body"} → 200 {"thread"}
func (h *handler) updateDiscussionThread(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	body, ok := decodeEditBody(w, r)
	if !ok {
		return
	}
	d, t, ok := h.requireDiscussionThread(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.UpdateDiscussionThread(r.Context(), d.ID, t.ID, body); err != nil {
		h.storeError(w, err)
		return
	}
	if t := h.findDiscussionThread(r, d.ID, t.ID); t != nil {
		writeJSON(w, http.StatusOK, map[string]any{"thread": threadDTO(t)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/w/{ws}/discussions/{id}/threads/{tid} → 204
func (h *handler) deleteDiscussionThread(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	d, t, ok := h.requireDiscussionThread(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.DeleteDiscussionThread(r.Context(), d.ID, t.ID); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/w/{ws}/discussions/{id}/threads/{tid}/answer {"answer": bool}
// → 200 {"thread"} — mark/unmark the thread as the discussion's answer;
// the store moves the mark and 422s non-qa categories (ErrNotAnswerable).
func (h *handler) setDiscussionThreadAnswer(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	var in struct {
		Answer *bool `json:"answer"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if in.Answer == nil {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"answer": "Answer must be true or false."})
		return
	}
	d, t, ok := h.requireDiscussionThread(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.SetThreadAnswer(r.Context(), d.ID, t.ID, *in.Answer); err != nil {
		h.storeError(w, err)
		return
	}
	if t := h.findDiscussionThread(r, d.ID, t.ID); t != nil {
		writeJSON(w, http.StatusOK, map[string]any{"thread": threadDTO(t)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST .../threads/{tid}/hide {"reason"} → 200 {"thread"}
func (h *handler) hideDiscussionThread(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	h.setDiscussionThreadHidden(w, r, ws, true)
}

// POST .../threads/{tid}/unhide → 200 {"thread"}
func (h *handler) unhideDiscussionThread(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	h.setDiscussionThreadHidden(w, r, ws, false)
}

func (h *handler) setDiscussionThreadHidden(w http.ResponseWriter, r *http.Request, ws *store.Workspace, hide bool) {
	reason := ""
	if hide {
		var ok bool
		if reason, ok = decodeHideBody(w, r); !ok {
			return
		}
	}
	d, t, ok := h.requireDiscussionThread(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.SetDiscussionThreadHidden(r.Context(), d.ID, t.ID, reason); err != nil {
		h.storeError(w, err)
		return
	}
	if t := h.findDiscussionThread(r, d.ID, t.ID); t != nil {
		writeJSON(w, http.StatusOK, map[string]any{"thread": threadDTO(t)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/w/{ws}/discussions/{id}/threads/{tid}/comments {"body"} →
// 201 {"reply"} — a reply inside an existing thread.
func (h *handler) createDiscussionReply(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	body, ok := decodeEditBody(w, r)
	if !ok {
		return
	}
	d, t, ok := h.requireDiscussionThread(w, r, ws)
	if !ok {
		return
	}
	cid, err := h.store.CreateDiscussionComment(r.Context(), d.ID, t.ID, body, actorID(r))
	if err != nil {
		h.storeError(w, err)
		return
	}
	if t := h.findDiscussionThread(r, d.ID, t.ID); t != nil {
		for _, c := range t.Comments {
			if c.ID == cid {
				writeJSON(w, http.StatusCreated, map[string]any{"reply": replyDTO(c)})
				return
			}
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"reply": map[string]any{"id": cid}})
}

// PATCH /api/v1/w/{ws}/discussions/{id}/threads/{tid}/comments/{cid} → 200 {"reply"}
func (h *handler) updateDiscussionReply(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	body, ok := decodeEditBody(w, r)
	if !ok {
		return
	}
	t, c, ok := h.requireDiscussionReply(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.UpdateDiscussionComment(r.Context(), t.DiscussionID, t.ID, c.ID, body); err != nil {
		h.storeError(w, err)
		return
	}
	if t := h.findDiscussionThread(r, t.DiscussionID, t.ID); t != nil {
		for _, c2 := range t.Comments {
			if c2.ID == c.ID {
				writeJSON(w, http.StatusOK, map[string]any{"reply": replyDTO(c2)})
				return
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/w/{ws}/discussions/{id}/threads/{tid}/comments/{cid} → 204
func (h *handler) deleteDiscussionReply(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	t, c, ok := h.requireDiscussionReply(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.DeleteDiscussionComment(r.Context(), t.DiscussionID, t.ID, c.ID); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST .../comments/{cid}/hide {"reason"} → 200 {"reply"}
func (h *handler) hideDiscussionReply(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	h.setDiscussionReplyHidden(w, r, ws, true)
}

// POST .../comments/{cid}/unhide → 200 {"reply"}
func (h *handler) unhideDiscussionReply(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	h.setDiscussionReplyHidden(w, r, ws, false)
}

func (h *handler) setDiscussionReplyHidden(w http.ResponseWriter, r *http.Request, ws *store.Workspace, hide bool) {
	reason := ""
	if hide {
		var ok bool
		if reason, ok = decodeHideBody(w, r); !ok {
			return
		}
	}
	t, c, ok := h.requireDiscussionReply(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.SetDiscussionCommentHidden(r.Context(), t.DiscussionID, t.ID, c.ID, reason); err != nil {
		h.storeError(w, err)
		return
	}
	if t := h.findDiscussionThread(r, t.DiscussionID, t.ID); t != nil {
		for _, c2 := range t.Comments {
			if c2.ID == c.ID {
				writeJSON(w, http.StatusOK, map[string]any{"reply": replyDTO(c2)})
				return
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
