// Pull request endpoints → store domain types. PRs are
// workspace-independent, so these paths never carry the /w/{slug}
// segment — wsPath's empty-ws behavior already covers that.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/podhmo/go-spa-sample/internal/api"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// PullView is the bundled detail response — the PR row plus comments,
// head-side commits, and the live mergeability verdict.
type PullView struct {
	PR        *store.PullRequest
	Comments  []*store.PRComment
	Commits   []api.PRCommit
	Mergeable *PullMergeable
}

// PullMergeable mirrors the API's mergeable object on GET /pulls/{id}.
type PullMergeable struct {
	Status string   `json:"status"` // "clean" | "conflicting" | "unknown"
	Files  []string `json:"files,omitempty"`
}

// ListPullRequests maps the filter onto the list endpoint's query.
func (c *Client) ListPullRequests(ctx context.Context, f store.PRListFilter) ([]*store.PullRequest, int, error) {
	q := url.Values{}
	status := f.Status
	if status == "" {
		status = store.PRStatusOpen
	}
	q.Set("status", status)
	if f.Query != "" {
		q.Set("q", f.Query)
	}
	if f.Sort != "" {
		q.Set("sort", f.Sort)
	}
	return listWindow(ctx, f.Page, f.PerPage,
		func(apiPage, apiPerPage int) ([]*store.PullRequest, int, error) {
			q.Set("page", strconv.Itoa(apiPage))
			if apiPerPage > 0 {
				q.Set("per_page", strconv.Itoa(apiPerPage))
			} else {
				q.Del("per_page")
			}
			var res struct {
				Pulls []api.PullRequest `json:"pulls"`
				Total int               `json:"total"`
			}
			if err := c.do(ctx, "GET", "/pulls?"+q.Encode(), nil, &res); err != nil {
				return nil, 0, err
			}
			pulls := make([]*store.PullRequest, 0, len(res.Pulls))
			for _, p := range res.Pulls {
				pulls = append(pulls, pullRequestFromDTO(p))
			}
			return pulls, res.Total, nil
		})
}

// PullView fetches GET /pulls/{id} — PR, comments, commits and the
// mergeability probe in one response.
func (c *Client) PullView(ctx context.Context, id int64) (*PullView, error) {
	var res struct {
		PR        api.PullRequest `json:"pull_request"`
		Comments  []api.PRComment `json:"comments"`
		Commits   []api.PRCommit  `json:"commits"`
		Mergeable *PullMergeable  `json:"mergeable"`
	}
	if err := c.do(ctx, "GET", "/pulls/"+fmt.Sprint(id), nil, &res); err != nil {
		return nil, err
	}
	out := &PullView{
		PR:        pullRequestFromDTO(res.PR),
		Commits:   res.Commits,
		Mergeable: res.Mergeable,
	}
	for _, cm := range res.Comments {
		out.Comments = append(out.Comments, prCommentFromDTO(cm, id))
	}
	return out, nil
}

func (c *Client) GetPullRequest(ctx context.Context, id int64) (*store.PullRequest, error) {
	v, err := c.PullView(ctx, id)
	if err != nil {
		return nil, err
	}
	return v.PR, nil
}

// CreatePullRequest posts base/head/title/body; the server normalizes the
// refs and resolves the shas. authorID is ignored (the token decides).
func (c *Client) CreatePullRequest(ctx context.Context, in store.PRInput, _ int64, _ func(int64) error) (int64, error) {
	var res struct {
		PR api.PullRequest `json:"pull_request"`
	}
	err := c.do(ctx, "POST", "/pulls", map[string]any{
		"base": in.BaseRef, "head": in.HeadRef, "title": in.Title, "body": in.Body,
	}, &res)
	if err != nil {
		return 0, err
	}
	return res.PR.ID, nil
}

func (c *Client) UpdatePullRequest(ctx context.Context, id int64, in store.PRInput) error {
	return c.do(ctx, "PATCH", "/pulls/"+fmt.Sprint(id), map[string]any{
		"title": in.Title, "body": in.Body,
	}, nil)
}

func (c *Client) DeletePullRequest(ctx context.Context, id int64) error {
	return c.do(ctx, "DELETE", "/pulls/"+fmt.Sprint(id), nil, nil)
}

// SetPullStatus posts {"status":"closed"|"open"} — close/reopen verbs
// map to this in the CLI.
func (c *Client) SetPullStatus(ctx context.Context, id int64, status string) error {
	return c.do(ctx, "POST", "/pulls/"+fmt.Sprint(id)+"/status",
		map[string]any{"status": status}, nil)
}

// MergePullRequest posts the merge; the returned PR carries merge_sha
// and merge_method.
func (c *Client) MergePullRequest(ctx context.Context, id int64, method, message string) (*store.PullRequest, error) {
	var res struct {
		PR api.PullRequest `json:"pull_request"`
	}
	err := c.do(ctx, "POST", "/pulls/"+fmt.Sprint(id)+"/merge",
		map[string]any{"method": method, "message": message}, &res)
	if err != nil {
		return nil, err
	}
	return pullRequestFromDTO(res.PR), nil
}

func (c *Client) ListPRComments(ctx context.Context, prID int64) ([]*store.PRComment, error) {
	var res struct {
		Comments []api.PRComment `json:"comments"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/pulls/%d/comments", prID), nil, &res); err != nil {
		return nil, err
	}
	out := make([]*store.PRComment, 0, len(res.Comments))
	for _, cm := range res.Comments {
		out = append(out, prCommentFromDTO(cm, prID))
	}
	return out, nil
}

// CreatePRComment posts a conversation comment (in.Path == "") or an
// anchored review comment (in.Path+Line set, optional Side).
func (c *Client) CreatePRComment(ctx context.Context, prID int64, in store.PRCommentInput, _ int64) (int64, error) {
	var res struct {
		Comment api.PRComment `json:"comment"`
	}
	body := map[string]any{"body": in.Body}
	if in.Path != "" {
		body["path"] = in.Path
		body["line"] = in.Line
		if in.Side != "" {
			body["side"] = in.Side
		}
	}
	err := c.do(ctx, "POST", fmt.Sprintf("/pulls/%d/comments", prID), body, &res)
	if err != nil {
		return 0, err
	}
	return res.Comment.ID, nil
}

func (c *Client) UpdatePRComment(ctx context.Context, prID, commentID int64, body string) error {
	return c.do(ctx, "PATCH", fmt.Sprintf("/pulls/%d/comments/%d", prID, commentID),
		map[string]any{"body": body}, nil)
}

func (c *Client) DeletePRComment(ctx context.Context, prID, commentID int64) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/pulls/%d/comments/%d", prID, commentID), nil, nil)
}

func (c *Client) SetPRCommentHidden(ctx context.Context, prID, commentID int64, reason string) error {
	action := "unhide"
	in := any(nil)
	if reason != "" {
		action = "hide"
		in = map[string]any{"reason": reason}
	}
	return c.do(ctx, "POST", fmt.Sprintf("/pulls/%d/comments/%d/%s", prID, commentID, action), in, nil)
}

// PullDiff returns GET /pulls/{id}/diff — a unified diff as plain text.
func (c *Client) PullDiff(ctx context.Context, id int64) (string, error) {
	text, _, err := c.getText(ctx, fmt.Sprintf("/pulls/%d/diff", id))
	return text, err
}

// PullDiffTruncated is PullDiff with the server's X-Diff-Truncated flag:
// when the response exceeded the server's output cap the caller should
// warn rather than print a partial patch as if complete.
func (c *Client) PullDiffTruncated(ctx context.Context, id int64) (string, bool, error) {
	return c.getText(ctx, fmt.Sprintf("/pulls/%d/diff", id))
}

// PullMarkdown returns GET /pulls/{id}/markdown — the export rendered
// by internal/export.
func (c *Client) PullMarkdown(ctx context.Context, id int64) (string, error) {
	text, _, err := c.getText(ctx, fmt.Sprintf("/pulls/%d/markdown", id))
	return text, err
}

// getText reads a non-JSON endpoint; non-2xx becomes *Error like do().
func (c *Client) getText(ctx context.Context, path string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if err != nil {
		return "", false, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("GET %s: %w", req.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var e struct {
			Err struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		ae := &Error{Status: resp.StatusCode, Message: resp.Status}
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Err.Message != "" {
			ae.Message = e.Err.Message
		}
		return "", false, ae
	}
	raw, err := io.ReadAll(resp.Body)
	return string(raw), resp.Header.Get("X-Diff-Truncated") == "true", err
}

func pullRequestFromDTO(p api.PullRequest) *store.PullRequest {
	pr := &store.PullRequest{
		ID: p.ID, Title: p.Title, Body: p.Body, Status: p.Status,
		BaseRef: p.Base, HeadRef: p.Head, HeadSHA: p.HeadSHA,
		MergeSHA: p.MergeSHA, MergeMethod: p.MergeMethod,
		Author: userFromDTO(p.Author),
	}
	if t, err := time.Parse(time.RFC3339, p.CreatedAt); err == nil {
		pr.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, p.UpdatedAt); err == nil {
		pr.UpdatedAt = t
	}
	if p.ClosedAt != nil {
		if t, err := time.Parse(time.RFC3339, *p.ClosedAt); err == nil {
			pr.ClosedAt = &t
		}
	}
	return pr
}

func prCommentFromDTO(c api.PRComment, prID int64) *store.PRComment {
	out := &store.PRComment{
		ID: c.ID, PRID: prID, Body: c.Body,
		Path: c.Path, Line: c.Line, Side: c.Side,
		AnchorSHA: c.AnchorSHA, DiffSnippet: c.DiffSnippet,
		Author:     userFromDTO(c.Author),
		Moderation: moderationFromDTO(c.Hidden, c.HiddenReason, c.EditedAt),
	}
	if t, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
		out.CreatedAt = t
	}
	return out
}

// Backend-shaped adapters — the remote server resolves its own shas on
// close/reopen, so the local-mode arguments are ignored here.
func (c *Client) ClosePullRequest(ctx context.Context, id int64, _, _ string) error {
	return c.SetPullStatus(ctx, id, "closed")
}

func (c *Client) ReopenPullRequest(ctx context.Context, id int64) error {
	return c.SetPullStatus(ctx, id, "open")
}

// MarkMerged has no remote equivalent: remote merges go through
// MergePullRequest, which does the git work and records the result
// server-side. It exists only to satisfy the shared Backend contract.
func (c *Client) MarkMerged(ctx context.Context, id int64, _ store.PRMergeInfo) error {
	return fmt.Errorf("client: MarkMerged is not a remote operation; use MergePullRequest")
}
