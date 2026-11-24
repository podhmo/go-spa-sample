// ScopedStore is a workspace-bound view of Store used by the local CLI
// backend: every issue/discussion operation is confined to one workspace
// so a local `--workspace` behaves like the remote API's /w/{slug}/...
// routes (out-of-scope ids answer ErrNotFound). Workspace management
// methods pass straight through.
package store

import (
	"context"
)

type ScopedStore struct {
	s    *Store
	wsID int64
}

// In resolves the workspace slug eagerly — a miss is ErrNotFound — and
// returns the scoped view. An empty slug returns an unscoped view (wsID 0)
// for workspace-management callers; resource operations then miss every
// scope check and answer ErrNotFound.
func (s *Store) In(ctx context.Context, slug string) (*ScopedStore, error) {
	if slug == "" {
		return &ScopedStore{s: s, wsID: 0}, nil
	}
	ws, err := s.WorkspaceBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return &ScopedStore{s: s, wsID: ws.ID}, nil
}

// inScope is ErrNotFound when id names an issue of another workspace.
func (sc *ScopedStore) inScope(ctx context.Context, id int64) error {
	i, err := sc.s.GetIssue(ctx, id)
	if err != nil {
		return err
	}
	if i.WorkspaceID != sc.wsID {
		return ErrNotFound
	}
	return nil
}

func (sc *ScopedStore) inScopeDiscussion(ctx context.Context, id int64) error {
	d, err := sc.s.GetDiscussion(ctx, id)
	if err != nil {
		return err
	}
	if d.WorkspaceID != sc.wsID {
		return ErrNotFound
	}
	return nil
}

func (sc *ScopedStore) ListIssues(ctx context.Context, f ListFilter) ([]*Issue, int, error) {
	return sc.s.ListIssues(ctx, sc.wsID, f)
}

func (sc *ScopedStore) GetIssue(ctx context.Context, id int64) (*Issue, error) {
	i, err := sc.s.GetIssue(ctx, id)
	if err != nil {
		return nil, err
	}
	if i.WorkspaceID != sc.wsID {
		return nil, ErrNotFound
	}
	return i, nil
}

func (sc *ScopedStore) CreateIssue(ctx context.Context, in IssueInput, authorID int64) (int64, error) {
	return sc.s.CreateIssue(ctx, sc.wsID, in, authorID)
}

func (sc *ScopedStore) UpdateIssue(ctx context.Context, id int64, in IssueInput) error {
	if err := sc.inScope(ctx, id); err != nil {
		return err
	}
	return sc.s.UpdateIssue(ctx, id, in)
}

func (sc *ScopedStore) SetIssueStatus(ctx context.Context, id int64, status string) error {
	if err := sc.inScope(ctx, id); err != nil {
		return err
	}
	return sc.s.SetIssueStatus(ctx, id, status)
}

func (sc *ScopedStore) DeleteIssue(ctx context.Context, id int64) error {
	if err := sc.inScope(ctx, id); err != nil {
		return err
	}
	return sc.s.DeleteIssue(ctx, id)
}

func (sc *ScopedStore) ListComments(ctx context.Context, issueID int64) ([]*Comment, error) {
	if err := sc.inScope(ctx, issueID); err != nil {
		return nil, err
	}
	return sc.s.ListComments(ctx, issueID)
}

func (sc *ScopedStore) CreateComment(ctx context.Context, issueID int64, body string, authorID int64) (int64, error) {
	if err := sc.inScope(ctx, issueID); err != nil {
		return 0, err
	}
	return sc.s.CreateComment(ctx, issueID, body, authorID)
}

// Comment moderation passes through after the issue scope check — the
// store methods then confine the row to that issue id, so a comment of
// another issue answers ErrNotFound.

func (sc *ScopedStore) UpdateComment(ctx context.Context, issueID, commentID int64, body string) error {
	if err := sc.inScope(ctx, issueID); err != nil {
		return err
	}
	return sc.s.UpdateComment(ctx, issueID, commentID, body)
}

func (sc *ScopedStore) DeleteComment(ctx context.Context, issueID, commentID int64) error {
	if err := sc.inScope(ctx, issueID); err != nil {
		return err
	}
	return sc.s.DeleteComment(ctx, issueID, commentID)
}

func (sc *ScopedStore) SetCommentHidden(ctx context.Context, issueID, commentID int64, reason string) error {
	if err := sc.inScope(ctx, issueID); err != nil {
		return err
	}
	return sc.s.SetCommentHidden(ctx, issueID, commentID, reason)
}

// ListLabels returns the labels of kind visible in this workspace: master
// rows plus its own. Kind is ""-safe — pass KindLabelIssue/Discussion.
func (sc *ScopedStore) ListLabels(ctx context.Context, kind string) ([]*Label, error) {
	return sc.s.ListLabels(ctx, sc.wsID, kind)
}

// ListArchivedLabels is the archived counterpart of ListLabels.
func (sc *ScopedStore) ListArchivedLabels(ctx context.Context, kind string) ([]*Label, error) {
	return sc.s.ListArchivedLabels(ctx, sc.wsID, kind)
}

// inScopeLabel is ErrNotFound when id names a label of another workspace;
// master labels (WorkspaceID 0) are in scope from any workspace.
func (sc *ScopedStore) inScopeLabel(ctx context.Context, id int64) error {
	l, err := sc.s.GetLabel(ctx, id)
	if err != nil {
		return err
	}
	if l.WorkspaceID != 0 && l.WorkspaceID != sc.wsID {
		return ErrNotFound
	}
	return nil
}

func (sc *ScopedStore) GetLabel(ctx context.Context, id int64) (*Label, error) {
	l, err := sc.s.GetLabel(ctx, id)
	if err != nil {
		return nil, err
	}
	if l.WorkspaceID != 0 && l.WorkspaceID != sc.wsID {
		return nil, ErrNotFound
	}
	return l, nil
}

// CreateLabel adds a label to this workspace, or to the shared master
// data when master is true.
func (sc *ScopedStore) CreateLabel(ctx context.Context, kind string, in LabelInput, master bool) (int64, error) {
	wsID := sc.wsID
	if master {
		wsID = 0
	}
	return sc.s.CreateLabel(ctx, wsID, kind, in)
}

func (sc *ScopedStore) UpdateLabel(ctx context.Context, id int64, in LabelInput) error {
	if err := sc.inScopeLabel(ctx, id); err != nil {
		return err
	}
	return sc.s.UpdateLabel(ctx, id, in)
}

func (sc *ScopedStore) ArchiveLabel(ctx context.Context, id int64, archived bool) error {
	if err := sc.inScopeLabel(ctx, id); err != nil {
		return err
	}
	return sc.s.ArchiveLabel(ctx, id, archived)
}

func (sc *ScopedStore) ListDiscussions(ctx context.Context, f DiscussionListFilter) ([]*Discussion, int, error) {
	return sc.s.ListDiscussions(ctx, sc.wsID, f)
}

func (sc *ScopedStore) GetDiscussion(ctx context.Context, id int64) (*Discussion, error) {
	d, err := sc.s.GetDiscussion(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.WorkspaceID != sc.wsID {
		return nil, ErrNotFound
	}
	return d, nil
}

func (sc *ScopedStore) CreateDiscussion(ctx context.Context, in DiscussionInput, authorID int64) (int64, error) {
	return sc.s.CreateDiscussion(ctx, sc.wsID, in, authorID)
}

func (sc *ScopedStore) UpdateDiscussion(ctx context.Context, id int64, in DiscussionInput) error {
	if err := sc.inScopeDiscussion(ctx, id); err != nil {
		return err
	}
	return sc.s.UpdateDiscussion(ctx, id, in)
}

func (sc *ScopedStore) DeleteDiscussion(ctx context.Context, id int64) error {
	if err := sc.inScopeDiscussion(ctx, id); err != nil {
		return err
	}
	return sc.s.DeleteDiscussion(ctx, id)
}

func (sc *ScopedStore) ListDiscussionThreads(ctx context.Context, discussionID int64) ([]*DiscussionThread, error) {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return nil, err
	}
	return sc.s.ListDiscussionThreads(ctx, discussionID)
}

func (sc *ScopedStore) CreateDiscussionThread(ctx context.Context, discussionID int64, body string, authorID int64) (int64, error) {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return 0, err
	}
	return sc.s.CreateDiscussionThread(ctx, discussionID, body, authorID)
}

func (sc *ScopedStore) CreateDiscussionComment(ctx context.Context, discussionID, threadID int64, body string, authorID int64) (int64, error) {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return 0, err
	}
	return sc.s.CreateDiscussionComment(ctx, discussionID, threadID, body, authorID)
}

func (sc *ScopedStore) SetThreadAnswer(ctx context.Context, discussionID, threadID int64, answer bool) error {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return err
	}
	return sc.s.SetThreadAnswer(ctx, discussionID, threadID, answer)
}

// Thread and reply moderation passes through after the discussion scope
// check; the store methods additionally verify the discussion→thread→
// reply chain, so mismatched ids answer ErrNotFound.

func (sc *ScopedStore) UpdateDiscussionThread(ctx context.Context, discussionID, threadID int64, body string) error {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return err
	}
	return sc.s.UpdateDiscussionThread(ctx, discussionID, threadID, body)
}

func (sc *ScopedStore) DeleteDiscussionThread(ctx context.Context, discussionID, threadID int64) error {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return err
	}
	return sc.s.DeleteDiscussionThread(ctx, discussionID, threadID)
}

func (sc *ScopedStore) SetDiscussionThreadHidden(ctx context.Context, discussionID, threadID int64, reason string) error {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return err
	}
	return sc.s.SetDiscussionThreadHidden(ctx, discussionID, threadID, reason)
}

func (sc *ScopedStore) UpdateDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64, body string) error {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return err
	}
	return sc.s.UpdateDiscussionComment(ctx, discussionID, threadID, commentID, body)
}

func (sc *ScopedStore) DeleteDiscussionComment(ctx context.Context, discussionID, threadID, commentID int64) error {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return err
	}
	return sc.s.DeleteDiscussionComment(ctx, discussionID, threadID, commentID)
}

func (sc *ScopedStore) SetDiscussionCommentHidden(ctx context.Context, discussionID, threadID, commentID int64, reason string) error {
	if err := sc.inScopeDiscussion(ctx, discussionID); err != nil {
		return err
	}
	return sc.s.SetDiscussionCommentHidden(ctx, discussionID, threadID, commentID, reason)
}

func (sc *ScopedStore) ListDiscussionCategories(ctx context.Context) ([]*DiscussionCategory, error) {
	return sc.s.ListDiscussionCategories(ctx)
}

// Workspace management methods pass through unscoped.

func (sc *ScopedStore) ListWorkspaces(ctx context.Context) ([]*Workspace, error) {
	return sc.s.ListWorkspaces(ctx)
}

func (sc *ScopedStore) WorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error) {
	return sc.s.WorkspaceBySlug(ctx, slug)
}

func (sc *ScopedStore) CreateWorkspace(ctx context.Context, in WorkspaceInput) (int64, error) {
	return sc.s.CreateWorkspace(ctx, in)
}

func (sc *ScopedStore) DeleteWorkspace(ctx context.Context, slug string) error {
	return sc.s.DeleteWorkspace(ctx, slug)
}

// Pull requests are workspace-independent — the scoped view passes them
// through unscoped (docs/sketch/ja/pull-requests.md §6).
func (sc *ScopedStore) ListPullRequests(ctx context.Context, f PRListFilter) ([]*PullRequest, int, error) {
	return sc.s.ListPullRequests(ctx, f)
}
func (sc *ScopedStore) GetPullRequest(ctx context.Context, id int64) (*PullRequest, error) {
	return sc.s.GetPullRequest(ctx, id)
}
func (sc *ScopedStore) CreatePullRequest(ctx context.Context, in PRInput, authorID int64, pin func(int64) error) (int64, error) {
	return sc.s.CreatePullRequest(ctx, in, authorID, pin)
}
func (sc *ScopedStore) UpdatePullRequest(ctx context.Context, id int64, in PRInput) error {
	return sc.s.UpdatePullRequest(ctx, id, in)
}
func (sc *ScopedStore) DeletePullRequest(ctx context.Context, id int64) error {
	return sc.s.DeletePullRequest(ctx, id)
}
func (sc *ScopedStore) ClosePullRequest(ctx context.Context, id int64, headSHA, mergeBaseSHA string) error {
	return sc.s.ClosePullRequest(ctx, id, headSHA, mergeBaseSHA)
}
func (sc *ScopedStore) ReopenPullRequest(ctx context.Context, id int64) error {
	return sc.s.ReopenPullRequest(ctx, id)
}
func (sc *ScopedStore) MarkMerged(ctx context.Context, id int64, mi PRMergeInfo) error {
	return sc.s.MarkMerged(ctx, id, mi)
}
func (sc *ScopedStore) ListPRComments(ctx context.Context, prID int64) ([]*PRComment, error) {
	return sc.s.ListPRComments(ctx, prID)
}
func (sc *ScopedStore) CreatePRComment(ctx context.Context, prID int64, in PRCommentInput, authorID int64) (int64, error) {
	return sc.s.CreatePRComment(ctx, prID, in, authorID)
}
func (sc *ScopedStore) UpdatePRComment(ctx context.Context, prID, commentID int64, body string) error {
	return sc.s.UpdatePRComment(ctx, prID, commentID, body)
}
func (sc *ScopedStore) DeletePRComment(ctx context.Context, prID, commentID int64) error {
	return sc.s.DeletePRComment(ctx, prID, commentID)
}
func (sc *ScopedStore) SetPRCommentHidden(ctx context.Context, prID, commentID int64, reason string) error {
	return sc.s.SetPRCommentHidden(ctx, prID, commentID, reason)
}
