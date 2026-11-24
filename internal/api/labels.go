// Label endpoints under /api/v1/w/{ws}/labels. Labels are workspace-scoped
// like issues and discussions: the union of shared master rows and the
// workspace's own. Deleting archives the row rather than removing it —
// the label stays attached to the issues/discussions that carry it.
package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/store"
)

// GET /api/v1/w/{ws}/labels?kind=issue|discussion → {"labels": [...]}
// kind filters; absent returns both kinds.
func (h *handler) listLabels(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	kind := r.URL.Query().Get("kind")
	out := []Label{}
	kinds := []string{kind}
	if kind == "" {
		kinds = []string{store.KindLabelIssue, store.KindLabelDiscussion}
	}
	for _, k := range kinds {
		if k != store.KindLabelIssue && k != store.KindLabelDiscussion {
			writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"kind": "Kind must be \"issue\" or \"discussion\"."})
			return
		}
		labels, err := h.store.ListLabels(r.Context(), ws.ID, k)
		if err != nil {
			h.storeError(w, err)
			return
		}
		for _, l := range labels {
			out = append(out, labelDTO(*l))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": out})
}

type labelBody struct {
	Name   *string `json:"name"`
	Color  *string `json:"color"`
	Kind   string  `json:"kind"`
	Shared bool    `json:"shared"` // true creates a master label visible in every workspace
}

// POST /api/v1/w/{ws}/labels {"name","color","kind","shared"} → 201
func (h *handler) createLabel(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	var in labelBody
	if !decodeBody(w, r, &in) {
		return
	}
	kind := in.Kind
	if kind == "" {
		kind = store.KindLabelIssue
	}
	if kind != store.KindLabelIssue && kind != store.KindLabelDiscussion {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"kind": "Kind must be \"issue\" or \"discussion\"."})
		return
	}
	input := store.LabelInput{
		Name:  strings.TrimSpace(deref(in.Name)),
		Color: strings.TrimSpace(deref(in.Color)),
	}
	if fe := store.ValidateLabelInput(input); len(fe) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	wsID := ws.ID
	if in.Shared {
		wsID = 0
	}
	id, err := h.store.CreateLabel(r.Context(), wsID, kind, input)
	if err != nil {
		h.labelStoreError(w, err)
		return
	}
	l, err := h.store.GetLabel(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"label": labelDTO(*l)})
}

// PATCH /api/v1/w/{ws}/labels/{id} {"name","color"} → 200. Kind and
// workspace scope are immutable.
func (h *handler) updateLabel(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	l, ok := h.labelInScope(w, r, ws)
	if !ok {
		return
	}
	var in labelBody
	if !decodeBody(w, r, &in) {
		return
	}
	input := store.LabelInput{Name: l.Name, Color: l.Color}
	if in.Name != nil {
		input.Name = strings.TrimSpace(*in.Name)
	}
	if in.Color != nil {
		input.Color = strings.TrimSpace(*in.Color)
	}
	if fe := store.ValidateLabelInput(input); len(fe) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	if err := h.store.UpdateLabel(r.Context(), l.ID, input); err != nil {
		h.labelStoreError(w, err)
		return
	}
	l, err := h.store.GetLabel(r.Context(), l.ID)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"label": labelDTO(*l)})
}

// DELETE /api/v1/w/{ws}/labels/{id} → 204. Archives the label: it leaves
// pickers and filters but stays attached to existing issues/discussions.
func (h *handler) deleteLabel(w http.ResponseWriter, r *http.Request, ws *store.Workspace) {
	l, ok := h.labelInScope(w, r, ws)
	if !ok {
		return
	}
	if err := h.store.ArchiveLabel(r.Context(), l.ID, true); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// labelInScope loads the {id} label and applies the same scope rule the
// HTML handlers use: master labels anywhere, workspace labels only in
// their own workspace.
func (h *handler) labelInScope(w http.ResponseWriter, r *http.Request, ws *store.Workspace) (*store.Label, bool) {
	id, ok := h.pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	l, err := h.store.GetLabel(r.Context(), id)
	if err != nil {
		h.storeError(w, err)
		return nil, false
	}
	if l.WorkspaceID != 0 && l.WorkspaceID != ws.ID {
		writeError(w, http.StatusNotFound, "Not found.", nil)
		return nil, false
	}
	return l, true
}

// labelStoreError maps label-specific store errors; everything else falls
// through to storeError.
func (h *handler) labelStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrDuplicateLabel) {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", map[string]string{"name": "A label with this name already exists in that scope."})
		return
	}
	h.storeError(w, err)
}
