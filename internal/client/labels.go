// Label endpoints → store domain types.
package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/podhmo/go-spa-sample/internal/api"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// ListLabels reads the workspace's label union (shared master rows plus
// its own) for the given kind.
func (c *Client) ListLabels(ctx context.Context, kind string) ([]*store.Label, error) {
	var res struct {
		Labels []api.Label `json:"labels"`
	}
	q := url.Values{"kind": {kind}}
	if err := c.do(ctx, "GET", c.wsPath("labels")+"?"+q.Encode(), nil, &res); err != nil {
		return nil, err
	}
	out := make([]*store.Label, 0, len(res.Labels))
	for _, l := range res.Labels {
		l := labelFromDTO(l)
		out = append(out, &l)
	}
	return out, nil
}

// CreateLabel posts a new label; master=true makes it a shared label
// visible in every workspace.
func (c *Client) CreateLabel(ctx context.Context, kind string, in store.LabelInput, master bool) (int64, error) {
	var res struct {
		Label api.Label `json:"label"`
	}
	err := c.do(ctx, "POST", c.wsPath("labels"), map[string]any{
		"name": in.Name, "color": in.Color, "kind": kind, "shared": master,
	}, &res)
	if err != nil {
		return 0, err
	}
	return res.Label.ID, nil
}

// ArchiveLabel deletes a label — the API archives it server-side, so
// archived=false re-creates go through CreateLabel instead (creating a
// name that matches an archived label restores it).
func (c *Client) ArchiveLabel(ctx context.Context, id int64, archived bool) error {
	if !archived {
		return fmt.Errorf("client: unarchiving is not supported; create the label again instead")
	}
	return c.do(ctx, "DELETE", c.wsPath("labels/")+fmt.Sprint(id), nil, nil)
}

func labelFromDTO(l api.Label) store.Label {
	out := store.Label{ID: l.ID, Name: l.Name, Color: l.Color, Kind: l.Kind, Archived: l.Archived}
	if l.WorkspaceID != nil {
		out.WorkspaceID = *l.WorkspaceID
	}
	return out
}
