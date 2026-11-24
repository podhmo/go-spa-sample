// Workspace endpoints: /api/v1/workspaces plus /api/v1/workspaces/{slug}.
// Resources inside a workspace live under /api/v1/w/{slug}/...
package api

import (
	"net/http"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/store"
)

type Workspace struct {
	ID              int64  `json:"id"`
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	IssueCount      int    `json:"issue_count"`
	DiscussionCount int    `json:"discussion_count"`
	CreatedAt       string `json:"created_at"`
}

func workspaceDTO(ws *store.Workspace) Workspace {
	return Workspace{
		ID: ws.ID, Slug: ws.Slug, Name: ws.Name, Description: ws.Description,
		IssueCount: ws.IssueCount, DiscussionCount: ws.DiscussionCount,
		CreatedAt: fmtTime(ws.CreatedAt),
	}
}

// GET /api/v1/workspaces → {"workspaces": [...]}
func (h *handler) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	wss, err := h.store.ListWorkspaces(r.Context())
	if err != nil {
		h.storeError(w, err)
		return
	}
	out := []Workspace{}
	for _, ws := range wss {
		out = append(out, workspaceDTO(ws))
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": out})
}

type workspaceBody struct {
	Slug        *string `json:"slug"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// POST /api/v1/workspaces {"slug", "name"?, "description"?} → 201
func (h *handler) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var in workspaceBody
	if !decodeBody(w, r, &in) {
		return
	}
	input := store.WorkspaceInput{
		Slug:        strings.TrimSpace(deref(in.Slug)),
		Name:        strings.TrimSpace(deref(in.Name)),
		Description: strings.TrimSpace(deref(in.Description)),
	}
	if fe := store.ValidateWorkspaceInput(input); fe.Any() {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields(fe))
		return
	}
	if _, err := h.store.CreateWorkspace(r.Context(), input); err != nil {
		h.storeError(w, err)
		return
	}
	ws, err := h.store.WorkspaceBySlug(r.Context(), input.Slug)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"workspace": workspaceDTO(ws)})
}

// GET /api/v1/workspaces/{slug} → {"workspace": {...}}
func (h *handler) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, err := h.store.WorkspaceBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": workspaceDTO(ws)})
}

// DELETE /api/v1/workspaces/{slug} → 204 — contents included.
func (h *handler) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteWorkspace(r.Context(), r.PathValue("slug")); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
