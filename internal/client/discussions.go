// Discussion endpoints → store domain types.
package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/podhmo/go-spa-sample/internal/api"
	"github.com/podhmo/go-spa-sample/internal/store"
)

// ListDiscussions maps the filter onto the list endpoint's query
// parameters (category is a slug, like the ?category= URL filter).
func (c *Client) ListDiscussions(ctx context.Context, f store.DiscussionListFilter) ([]*store.Discussion, int, error) {
	q := url.Values{}
	if f.CategorySlug != "" {
		q.Set("category", f.CategorySlug)
	}
	if f.Answered != "" {
		q.Set("answered", f.Answered)
	}
	if f.Query != "" {
		q.Set("q", f.Query)
	}
	if f.Sort != "" {
		q.Set("sort", f.Sort)
	}
	return listWindow(ctx, f.Page, f.PerPage,
		func(apiPage, apiPerPage int) ([]*store.Discussion, int, error) {
			q.Set("page", strconv.Itoa(apiPage))
			if apiPerPage > 0 {
				q.Set("per_page", strconv.Itoa(apiPerPage))
			} else {
				q.Del("per_page")
			}
			return c.listDiscussionsPage(ctx, q)
		})
}

func (c *Client) listDiscussionsPage(ctx context.Context, q url.Values) ([]*store.Discussion, int, error) {
	var res struct {
		Discussions []api.Discussion `json:"discussions"`
		Total       int              `json:"total"`
	}
	if err := c.do(ctx, "GET", c.wsPath("discussions?")+q.Encode(), nil, &res); err != nil {
		return nil, 0, err
	}
	out := make([]*store.Discussion, 0, len(res.Discussions))
	for _, d := range res.Discussions {
		out = append(out, discussionFromDTO(d))
	}
	return out, res.Total, nil
}

func (c *Client) GetDiscussion(ctx context.Context, id int64) (*store.Discussion, error) {
	var res struct {
		Discussion api.Discussion `json:"discussion"`
	}
	if err := c.do(ctx, "GET", c.wsPath("discussions/")+fmt.Sprint(id), nil, &res); err != nil {
		return nil, err
	}
	return discussionFromDTO(res.Discussion), nil
}

// ListDiscussionThreads reads threads off the detail endpoint — like
// ListComments there is no dedicated GET. A missing discussion
// surfaces as *Error{Status:404} where the local backend returns an
// empty list and no error.
func (c *Client) ListDiscussionThreads(ctx context.Context, discussionID int64) ([]*store.DiscussionThread, error) {
	var res struct {
		Threads []api.DiscussionThread `json:"threads"`
	}
	if err := c.do(ctx, "GET", c.wsPath("discussions/")+fmt.Sprint(discussionID), nil, &res); err != nil {
		return nil, err
	}
	out := make([]*store.DiscussionThread, 0, len(res.Threads))
	for _, t := range res.Threads {
		out = append(out, discussionThreadFromDTO(t, discussionID))
	}
	return out, nil
}

// CreateDiscussion posts the discussion; the server resolves the author
// from the token, so authorID is ignored here.
func (c *Client) CreateDiscussion(ctx context.Context, in store.DiscussionInput, _ int64) (int64, error) {
	var res struct {
		Discussion api.Discussion `json:"discussion"`
	}
	err := c.do(ctx, "POST", c.wsPath("discussions"), map[string]any{
		"title": in.Title, "body": in.Body, "category_id": in.CategoryID, "label_ids": in.LabelIDs,
	}, &res)
	if err != nil {
		return 0, err
	}
	return res.Discussion.ID, nil
}

func (c *Client) UpdateDiscussion(ctx context.Context, id int64, in store.DiscussionInput) error {
	labelIDs := in.LabelIDs
	if labelIDs == nil {
		// Same nil-means-clear convention as UpdateIssue.
		labelIDs = []int64{}
	}
	return c.do(ctx, "PATCH", c.wsPath("discussions/")+fmt.Sprint(id), map[string]any{
		"title": in.Title, "body": in.Body, "category_id": in.CategoryID, "label_ids": labelIDs,
	}, nil)
}

func (c *Client) DeleteDiscussion(ctx context.Context, id int64) error {
	return c.do(ctx, "DELETE", c.wsPath("discussions/")+fmt.Sprint(id), nil, nil)
}

func (c *Client) CreateDiscussionThread(ctx context.Context, discussionID int64, body string, _ int64) (int64, error) {
	var res struct {
		Thread api.DiscussionThread `json:"thread"`
	}
	err := c.do(ctx, "POST", c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads",
		map[string]any{"body": body}, &res)
	if err != nil {
		return 0, err
	}
	return res.Thread.ID, nil
}

func (c *Client) CreateDiscussionComment(ctx context.Context, discussionID, threadID int64, body string, _ int64) (int64, error) {
	var res struct {
		Reply api.DiscussionReply `json:"reply"`
	}
	err := c.do(ctx, "POST",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID)+"/comments",
		map[string]any{"body": body}, &res)
	if err != nil {
		return 0, err
	}
	return res.Reply.ID, nil
}

func (c *Client) SetThreadAnswer(ctx context.Context, discussionID, threadID int64, answer bool) error {
	return c.do(ctx, "POST",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID)+"/answer",
		map[string]any{"answer": answer}, nil)
}

// UpdateDiscussionThread PATCHes the thread's body (the opening post).
func (c *Client) UpdateDiscussionThread(ctx context.Context, discussionID, threadID int64, body string) error {
	return c.do(ctx, "PATCH",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID),
		map[string]any{"body": body}, nil)
}

// DeleteDiscussionThread removes the thread and its replies.
func (c *Client) DeleteDiscussionThread(ctx context.Context, discussionID, threadID int64) error {
	return c.do(ctx, "DELETE",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID), nil, nil)
}

// SetDiscussionThreadHidden minimizes the thread (reason "" restores).
func (c *Client) SetDiscussionThreadHidden(ctx context.Context, discussionID, threadID int64, reason string) error {
	action := "hide"
	var body map[string]any
	if reason != "" {
		body = map[string]any{"reason": reason}
	} else {
		action = "unhide"
	}
	return c.do(ctx, "POST",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID)+"/"+action,
		body, nil)
}

// UpdateDiscussionComment PATCHes a reply's body.
func (c *Client) UpdateDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64, body string) error {
	return c.do(ctx, "PATCH",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID)+"/comments/"+fmt.Sprint(commentID),
		map[string]any{"body": body}, nil)
}

// DeleteDiscussionComment removes a reply.
func (c *Client) DeleteDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64) error {
	return c.do(ctx, "DELETE",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID)+"/comments/"+fmt.Sprint(commentID),
		nil, nil)
}

// SetDiscussionCommentHidden minimizes a reply (reason "" restores).
func (c *Client) SetDiscussionCommentHidden(ctx context.Context, discussionID, threadID, commentID int64, reason string) error {
	action := "hide"
	var body map[string]any
	if reason != "" {
		body = map[string]any{"reason": reason}
	} else {
		action = "unhide"
	}
	return c.do(ctx, "POST",
		c.wsPath("discussions/")+fmt.Sprint(discussionID)+"/threads/"+fmt.Sprint(threadID)+"/comments/"+fmt.Sprint(commentID)+"/"+action,
		body, nil)
}

func (c *Client) ListDiscussionCategories(ctx context.Context) ([]*store.DiscussionCategory, error) {
	var res struct {
		Categories []api.DiscussionCategory `json:"categories"`
	}
	if err := c.do(ctx, "GET", "/discussion-categories", nil, &res); err != nil {
		return nil, err
	}
	out := make([]*store.DiscussionCategory, 0, len(res.Categories))
	for _, c := range res.Categories {
		c := c
		out = append(out, &store.DiscussionCategory{
			ID: c.ID, Name: c.Name, Slug: c.Slug,
			Description: c.Description, Format: c.Format,
		})
	}
	return out, nil
}

func discussionFromDTO(d api.Discussion) *store.Discussion {
	out := &store.Discussion{
		ID: d.ID, Title: d.Title, Body: d.Body,
		Category:  categoryFromDTO(d.Category),
		Author:    userFromDTO(d.Author),
		Answered:  d.Answered,
		CreatedAt: parseTime(d.CreatedAt), UpdatedAt: parseTime(d.UpdatedAt),
	}
	for _, l := range d.Labels {
		out.Labels = append(out.Labels, labelFromDTO(l))
	}
	return out
}

func discussionThreadFromDTO(t api.DiscussionThread, discussionID int64) *store.DiscussionThread {
	out := &store.DiscussionThread{
		ID: t.ID, DiscussionID: discussionID,
		IsAnswer: t.IsAnswer, Body: t.Body,
		Author:     userFromDTO(t.Author),
		CreatedAt:  parseTime(t.CreatedAt),
		Moderation: moderationFromDTO(t.Hidden, t.HiddenReason, t.EditedAt),
	}
	for _, c := range t.Comments {
		out.Comments = append(out.Comments, discussionCommentFromDTO(c, t.ID))
	}
	return out
}

func discussionCommentFromDTO(c api.DiscussionReply, threadID int64) *store.DiscussionComment {
	return &store.DiscussionComment{
		ID: c.ID, ThreadID: threadID, Body: c.Body,
		Author:     userFromDTO(c.Author),
		CreatedAt:  parseTime(c.CreatedAt),
		Moderation: moderationFromDTO(c.Hidden, c.HiddenReason, c.EditedAt),
	}
}

func categoryFromDTO(c api.DiscussionCategory) store.DiscussionCategory {
	return store.DiscussionCategory{
		ID: c.ID, Name: c.Name, Slug: c.Slug,
		Description: c.Description, Format: c.Format,
	}
}
