// Workspace management — unscoped like labels; the returned Workspace's
// Slug is what In() scopes a client to.
package client

import (
	"context"
	"net/url"

	"github.com/podhmo/go-spa-sample/internal/api"
	"github.com/podhmo/go-spa-sample/internal/store"
)

func (c *Client) ListWorkspaces(ctx context.Context) ([]*store.Workspace, error) {
	var res struct {
		Workspaces []api.Workspace `json:"workspaces"`
	}
	if err := c.do(ctx, "GET", "/workspaces", nil, &res); err != nil {
		return nil, err
	}
	out := []*store.Workspace{}
	for _, w := range res.Workspaces {
		out = append(out, workspaceFromDTO(w))
	}
	return out, nil
}

func (c *Client) WorkspaceBySlug(ctx context.Context, slug string) (*store.Workspace, error) {
	var res struct {
		Workspace api.Workspace `json:"workspace"`
	}
	if err := c.do(ctx, "GET", "/workspaces/"+url.PathEscape(slug), nil, &res); err != nil {
		return nil, err
	}
	return workspaceFromDTO(res.Workspace), nil
}

func (c *Client) CreateWorkspace(ctx context.Context, in store.WorkspaceInput) (int64, error) {
	var res struct {
		Workspace api.Workspace `json:"workspace"`
	}
	err := c.do(ctx, "POST", "/workspaces", map[string]any{
		"slug":        in.Slug,
		"name":        in.Name,
		"description": in.Description,
	}, &res)
	if err != nil {
		return 0, err
	}
	return res.Workspace.ID, nil
}

func (c *Client) DeleteWorkspace(ctx context.Context, slug string) error {
	return c.do(ctx, "DELETE", "/workspaces/"+url.PathEscape(slug), nil, nil)
}

func workspaceFromDTO(w api.Workspace) *store.Workspace {
	return &store.Workspace{
		ID:              w.ID,
		Slug:            w.Slug,
		Name:            w.Name,
		Description:     w.Description,
		IssueCount:      w.IssueCount,
		DiscussionCount: w.DiscussionCount,
		CreatedAt:       parseTime(w.CreatedAt),
	}
}
