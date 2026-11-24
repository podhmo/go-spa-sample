package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/export"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// GET /api/v1/issues?status=&q=&label=&label_id=&sort=&page=&per_page=
// Response: {"issues": [...], "total": N, "page": P, "per_page": PP}
// label_id filters by exact label id (preferred); label filters by name
// and is ambiguous when shared and workspace labels share a name.
func (h *handler) listIssues(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	q := r.URL.Query()
	f := store.ListFilter{
		Status:  q.Get("status"),
		Query:   strings.TrimSpace(q.Get("q")),
		Sort:    q.Get("sort"),
		Page:    queryInt(r, "page", 1),
		PerPage: min(queryInt(r, "per_page", defaultPerPage), maxPerPage),
	}
	if lid := q.Get("label_id"); lid != "" {
		f.LabelID = int64(queryInt(r, "label_id", 0))
	} else if name := q.Get("label"); name != "" {
		all, err := h.store.ListLabels(r.Context(), ws.ID, store.KindLabelIssue)
		if err != nil {
			h.storeError(w, err)
			return
		}
		for _, l := range all {
			if l.Name == name {
				f.LabelID = l.ID
				break
			}
		}
		if f.LabelID == 0 {
			writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"label": "Unknown label: " + name})
			return
		}
	}
	issues, total, err := h.store.ListIssues(r.Context(), ws.ID, f)
	if err != nil {
		h.storeError(w, err)
		return
	}
	out := []Issue{}
	for _, i := range issues {
		out = append(out, issueDTO(i))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issues": out, "total": total, "page": f.Page, "per_page": f.PerPage,
	})
}

// GET /api/v1/issues/{id} — one call returns the issue with labels plus
// its comments, matching the HTML detail page's shape.
func (h *handler) getIssue(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if !h.requireScope(w, r, ws, issue.WorkspaceID) {
		return
	}
	comments, err := h.store.ListComments(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	out := []Comment{}
	for _, c := range comments {
		out = append(out, commentDTO(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": issueDTO(issue), "comments": out})
}

// GET /api/v1/w/{ws}/issues/{id}/markdown → text/markdown — the issue
// with all comments rendered by internal/export. The HTML app's "Copy as
// markdown" cannot use this endpoint (Bearer only), so /w/{ws}/issues/
// {id}/markdown serves the same document on the cookie session.
func (h *handler) issueMarkdown(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if !h.requireScope(w, r, ws, issue.WorkspaceID) {
		return
	}
	comments, err := h.store.ListComments(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeMarkdown(w, fmt.Sprintf("issue-%d.md", id), export.IssueMarkdown(issue, comments, ws.Slug))
}

type issueBody struct {
	Title    *string  `json:"title"`
	Body     *string  `json:"body"`
	LabelIDs []int64  `json:"label_ids"`
	Labels   []string `json:"labels"` // label names, resolved via ListLabels
}

// POST /api/v1/issues {"title", "body", "label_ids"|"labels"} → 201
func (h *handler) createIssue(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	var in issueBody
	if !decodeBody(w, r, &in) {
		return
	}
	title := strings.TrimSpace(deref(in.Title))
	body := strings.TrimSpace(deref(in.Body))
	labelIDs, ferr, err := h.resolveLabelIDs(r, ws, store.KindLabelIssue, in.LabelIDs, in.Labels)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if ferr != nil {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", ferr)
		return
	}
	input := store.IssueInput{Title: title, Body: body, LabelIDs: labelIDs}
	if fe := store.ValidateIssueInput(input); fe.Any() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	id, err := h.store.CreateIssue(r.Context(), ws.ID, input, actorID(r))
	if err != nil {
		h.storeError(w, err)
		return
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"issue": issueDTO(issue)})
}

// PATCH /api/v1/issues/{id} — only fields present in the body are updated;
// absent fields keep their current values.
func (h *handler) updateIssue(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return
	}
	var in issueBody
	if !decodeBody(w, r, &in) {
		return
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if !h.requireScope(w, r, ws, issue.WorkspaceID) {
		return
	}
	input := store.IssueInput{Title: issue.Title, Body: issue.Body}
	for _, l := range issue.Labels {
		input.LabelIDs = append(input.LabelIDs, l.ID)
	}
	if in.Title != nil {
		input.Title = strings.TrimSpace(*in.Title)
	}
	if in.Body != nil {
		input.Body = strings.TrimSpace(*in.Body)
	}
	if in.LabelIDs != nil || in.Labels != nil {
		labelIDs, ferr, err := h.resolveLabelIDs(r, ws, store.KindLabelIssue, in.LabelIDs, in.Labels)
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
	if fe := store.ValidateIssueInput(input); fe.Any() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	if err := h.store.UpdateIssue(r.Context(), id, input); err != nil {
		h.storeError(w, err)
		return
	}
	issue, err = h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": issueDTO(issue)})
}

// POST /api/v1/issues/{id}/status {"status": "open"|"closed"} → 200.
// Status change stays a POST (not PATCH) so the wire mirrors the HTML app's
// explicit action endpoint.
func (h *handler) setIssueStatus(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if in.Status != store.StatusOpen && in.Status != store.StatusClosed {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"status": "Status must be \"open\" or \"closed\"."})
		return
	}
	issue0, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if !h.requireScope(w, r, ws, issue0.WorkspaceID) {
		return
	}
	if err := h.store.SetIssueStatus(r.Context(), id, in.Status); err != nil {
		h.storeError(w, err)
		return
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": issueDTO(issue)})
}

// DELETE /api/v1/issues/{id} → 204
func (h *handler) deleteIssue(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if !h.requireScope(w, r, ws, issue.WorkspaceID) {
		return
	}
	if err := h.store.DeleteIssue(r.Context(), id); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/issues/{id}/comments {"body"} → 201
func (h *handler) createComment(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	body := strings.TrimSpace(in.Body)
	if fe := store.ValidateCommentBody(body); fe.Code != "" {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"body": fe.Message})
		return
	}
	// GetIssue keeps a missing issue a 404 rather than a dangling comment
	// (the store also FK-guards, but the error would be generic).
	issue0, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if !h.requireScope(w, r, ws, issue0.WorkspaceID) {
		return
	}
	cid, err := h.store.CreateComment(r.Context(), id, body, actorID(r))
	if err != nil {
		h.storeError(w, err)
		return
	}
	comments, err := h.store.ListComments(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	for _, c := range comments {
		if c.ID == cid {
			writeJSON(w, http.StatusCreated, map[string]any{"comment": commentDTO(c)})
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"comment": map[string]any{"id": cid}})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type Label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Kind        string `json:"kind"`
	WorkspaceID *int64 `json:"workspace_id"` // null = shared master label
	Archived    bool   `json:"archived"`
}

type Issue struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Status    string  `json:"status"`
	Labels    []Label `json:"labels"`
	Author    *User   `json:"author"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type Comment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	Author    *User  `json:"author"`
	CreatedAt string `json:"created_at"`
	Moderation
}

func labelDTO(l store.Label) Label {
	out := Label{ID: l.ID, Name: l.Name, Color: l.Color, Kind: l.Kind, Archived: l.Archived}
	if l.WorkspaceID != 0 {
		id := l.WorkspaceID
		out.WorkspaceID = &id
	}
	return out
}

func issueDTO(i *store.Issue) Issue {
	out := Issue{
		ID: i.ID, Title: i.Title, Body: i.Body, Status: i.Status,
		Author: userDTO(i.Author), CreatedAt: fmtTime(i.CreatedAt), UpdatedAt: fmtTime(i.UpdatedAt),
		Labels: []Label{},
	}
	for _, l := range i.Labels {
		out.Labels = append(out.Labels, labelDTO(l))
	}
	return out
}

func commentDTO(c *store.Comment) Comment {
	return Comment{
		ID: c.ID, Body: c.Body, Author: userDTO(c.Author), CreatedAt: fmtTime(c.CreatedAt),
		Moderation: moderationDTO(c.Moderation),
	}
}

// resolveLabelIDs resolves label names ("labels") into ids, merged with
// explicit "label_ids". Unknown names are a 422 field error; a store
// failure is an error — silently dropping names would lose labels the
// client asked for.
func (h *handler) resolveLabelIDs(r *http.Request, ws *store.Workspace, kind string, ids []int64, names []string) ([]int64, map[string]string, error) {
	if len(names) == 0 {
		return ids, nil, nil
	}
	all, err := h.store.ListLabels(r.Context(), ws.ID, kind)
	if err != nil {
		return nil, nil, err
	}
	byName := map[string]int64{}
	for _, l := range all {
		byName[l.Name] = l.ID
	}
	for _, n := range names {
		id, ok := byName[n]
		if !ok {
			return nil, map[string]string{"labels": fmt.Sprintf("Unknown label: %s", n)}, nil
		}
		ids = append(ids, id)
	}
	return ids, nil, nil
}
