// Comment moderation endpoints — PATCH/DELETE on the post itself plus
// hide/unhide action POSTs, for issue comments. Each handler resolves
// the (resource, id) chain and the workspace scope first, so an id that
// does not line up is the same 404 as any missing resource.
package api

import (
	"net/http"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// hideReasonField is the shared 422 field map for an out-of-set reason
// — the allowed values ride along so CLI/API callers can self-correct.
func hideReasonField() map[string]string {
	return map[string]string{"reason": "Unknown hide reason. Valid values: " + strings.Join(store.HideReasons, ", ")}
}

// Moderation is the moderation triple embedded inline in the post
// DTOs: edited_at appears once the body changed, hidden +
// hidden_reason describe the minimized state. Exported so the CLI's
// --json views can build the same wire shape from store rows.
type Moderation struct {
	EditedAt     *string `json:"edited_at,omitempty"`
	Hidden       bool    `json:"hidden"`
	HiddenReason string  `json:"hidden_reason,omitempty"`
}

func moderationDTO(m store.Moderation) Moderation {
	var out Moderation
	if m.Edited() {
		s := fmtTime(m.EditedAt)
		out.EditedAt = &s
	}
	out.Hidden = m.Hidden
	out.HiddenReason = m.HiddenReason
	return out
}

// findIssueComment re-reads one comment after a mutation (the DTO the
// response carries); nil when the list cannot be read or the id is gone.
func (h *handler) findIssueComment(r *http.Request, issueID, commentID int64) *store.Comment {
	comments, err := h.store.ListComments(r.Context(), issueID)
	if err != nil {
		return nil
	}
	for _, c := range comments {
		if c.ID == commentID {
			return c
		}
	}
	return nil
}

// requireIssueComment resolves /issues/{id}/comments/{cid}: parent
// issue, workspace scope, and the comment itself. Any failure already
// wrote its response.
func (h *handler) requireIssueComment(w http.ResponseWriter, r *http.Request, ws *store.Workspace) (*store.Comment, bool) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	cid, ok := h.pathID(w, r, "cid")
	if !ok {
		return nil, false
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return nil, false
	}
	if !h.requireScope(w, ws, issue.WorkspaceID) {
		return nil, false
	}
	c := h.findIssueComment(r, id, cid)
	if c == nil {
		writeError(w, http.StatusNotFound, "Not found.", nil)
		return nil, false
	}
	return c, true
}

// decodeHideBody reads {"reason": "..."} for a hide POST; an invalid
// reason is a 422 — the store would also reject it, but the wire error
// names the field.
func decodeHideBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &in) {
		return "", false
	}
	if !store.ValidHideReason(in.Reason) {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", hideReasonField())
		return "", false
	}
	return in.Reason, true
}

// decodeEditBody reads {"body": "..."} for a moderation PATCH.
func decodeEditBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var in struct {
		Body string `json:"body"`
	}
	if !decodeBody(w, r, &in) {
		return "", false
	}
	body := strings.TrimSpace(in.Body)
	if fe := store.ValidateCommentBody(body); fe.Code != "" {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"body": fe.Message})
		return "", false
	}
	return body, true
}

// PATCH /api/v1/w/{ws}/issues/{id}/comments/{cid} {"body"} → 200 {"comment"}
// Like updateIssue the body decodes before the resource chain resolves:
// a malformed payload is a 400/422, not a 404.
func (h *handler) updateComment(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	body, ok := decodeEditBody(w, r)
	if !ok {
		return
	}
	c, ok := h.requireIssueComment(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.UpdateComment(r.Context(), c.IssueID, c.ID, body); err != nil {
		h.storeError(w, err)
		return
	}
	if c := h.findIssueComment(r, c.IssueID, c.ID); c != nil {
		writeJSON(w, http.StatusOK, map[string]any{"comment": commentDTO(c)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/w/{ws}/issues/{id}/comments/{cid} → 204
func (h *handler) deleteComment(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	c, ok := h.requireIssueComment(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.DeleteComment(r.Context(), c.IssueID, c.ID); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST .../comments/{cid}/hide {"reason"} → 200 {"comment"}
func (h *handler) hideComment(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	h.setCommentHidden(w, r, ws, true)
}

// POST .../comments/{cid}/unhide → 200 {"comment"}
func (h *handler) unhideComment(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	h.setCommentHidden(w, r, ws, false)
}

func (h *handler) setCommentHidden(w http.ResponseWriter, r *http.Request, ws *store.Workspace, hide bool) {
	reason := ""
	if hide {
		var ok bool
		if reason, ok = decodeHideBody(w, r); !ok {
			return
		}
	}
	c, ok := h.requireIssueComment(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.SetCommentHidden(r.Context(), c.IssueID, c.ID, reason); err != nil {
		h.storeError(w, err)
		return
	}
	if c := h.findIssueComment(r, c.IssueID, c.ID); c != nil {
		writeJSON(w, http.StatusOK, map[string]any{"comment": commentDTO(c)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
