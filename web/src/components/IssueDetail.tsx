import { Link } from '@tanstack/react-router'
import { useId, useReducer } from 'react'
import { HIDE_REASONS, type Comment, type Issue } from '../api'
import { ConfirmDialog } from './ConfirmDialog'
import { CopyButton } from './CopyButton'
import { submitOnCtrlEnter } from '../enter-submit'
import { fmtTime, labelStyle } from '../format'
import { StatusBadge } from './IssueList'
import { Markdown } from './Markdown'

type CommentUIState = { editing: boolean; draft: string }
type CommentUIAction =
  | { type: 'start-edit'; body: string }
  | { type: 'cancel' }
  | { type: 'draft'; value: string }

const commentUIReducer = (s: CommentUIState, a: CommentUIAction): CommentUIState => {
  switch (a.type) {
    case 'start-edit':
      return { editing: true, draft: a.body }
    case 'cancel':
      return { editing: false, draft: s.draft }
    case 'draft':
      return { ...s, draft: a.value }
  }
}

// Optional members are spread into CommentItem's props, so callers may
// legitimately pass explicit undefined (exactOptionalPropertyTypes).
export type CommentHandlers = {
  pending?: boolean | undefined
  onEdit?: ((cid: number, body: string) => void) | undefined
  onDelete?: ((cid: number) => void) | undefined
  onHide?: ((cid: number, reason: string) => void) | undefined
  onUnhide?: ((cid: number) => void) | undefined
}

// Hide reason picker: a native <dialog> holding a <form method="dialog"> —
// the select's value is read back on close, like ConfirmDialog's
// returnValue pattern.
function HideCommentDialog({
  commentId,
  pending,
  onHide,
}: {
  commentId: number
  pending?: boolean | undefined
  onHide: (reason: string) => void
}) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const dialogId = `hide-${id}`
  return (
    <>
      <button
        type="button"
        className="comment-action"
        {...({ commandfor: dialogId, command: 'show-modal' } as Record<string, string>)}
        disabled={pending}
        aria-label={`Hide comment ${commentId}`}
      >
        Hide
      </button>
      <dialog
        id={dialogId}
        className="confirm-dialog"
        aria-labelledby={`${dialogId}-title`}
        ref={(el) => {
          if (el && !el.hasAttribute('closedby')) el.setAttribute('closedby', 'any')
        }}
        onClose={(e) => {
          const d = e.currentTarget
          if (d.returnValue === 'hide') {
            const form = d.querySelector('form')
            const reason = String(new FormData(form ?? undefined).get('reason') ?? '')
            if (reason) onHide(reason)
          }
          d.returnValue = ''
        }}
      >
        <h2 id={`${dialogId}-title`}>Hide comment #{commentId}?</h2>
        <form method="dialog">
          <div className="field">
            <label htmlFor={`${dialogId}-reason`}>
              Reason{' '}
              <span className="req" aria-hidden="true">
                *
              </span>
            </label>
            <select
              id={`${dialogId}-reason`}
              name="reason"
              required
              data-error={`${dialogId}-reason-error`}
              defaultValue=""
            >
              <option value="" disabled>
                Select…
              </option>
              {HIDE_REASONS.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
            <span id={`${dialogId}-reason-error`} className="error-msg">
              A reason is required.
            </span>
          </div>
          <div className="form-actions">
            {/* Chrome validates required controls even on method="dialog"
                submits — formnovalidate lets Cancel close without a reason. */}
            <button type="submit" className="btn" value="cancel" formNoValidate autoFocus>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" value="hide">
              Hide comment
            </button>
          </div>
        </form>
      </dialog>
    </>
  )
}

function CommentItem({
  comment: c,
  pending,
  onEdit,
  onDelete,
  onHide,
  onUnhide,
}: { comment: Comment } & CommentHandlers) {
  const [ui, dispatch] = useReducer(commentUIReducer, { editing: false, draft: '' })
  const permalink = () => `${location.origin}${location.pathname}#comment-${c.id}`
  return (
    <li className={`card comment${c.hidden ? ' comment-hidden' : ''}`} id={`comment-${c.id}`}>
      <header className="comment-meta">
        <span className="comment-author">{c.author?.name ?? 'anonymous'} commented</span>{' '}
        <a className="comment-permalink" href={`#comment-${c.id}`}>
          <time dateTime={c.created_at}>{fmtTime(c.created_at)}</time>
        </a>
        {c.edited_at && (
          <>
            {' '}
            · <span className="comment-edited">edited</span>
          </>
        )}
        <CopyButton className="copy-btn" label="copy link" getText={permalink} pending={pending} />
      </header>
      {c.hidden ? (
        <>
          <p className="comment-hidden-note">
            This comment was marked as {c.hidden_reason ?? 'hidden'}.
          </p>
          <details className="comment-hidden-body">
            <summary>show comment</summary>
            <div className="comment-body">
              <Markdown text={c.body} />
            </div>
          </details>
        </>
      ) : ui.editing ? (
        <form
          aria-label={`edit comment ${c.id}`}
          onSubmit={(e) => {
            e.preventDefault()
            onEdit?.(c.id, ui.draft)
            dispatch({ type: 'cancel' })
          }}
        >
          <div className="field">
            <label htmlFor={`comment-edit-${c.id}`} className="visually-hidden">
              comment body
            </label>
            <textarea
              id={`comment-edit-${c.id}`}
              value={ui.draft}
              onChange={(e) => dispatch({ type: 'draft', value: e.target.value })}
              onKeyDown={submitOnCtrlEnter}
              rows={4}
              required
              aria-label="comment body"
            />
          </div>
          <div className="form-actions">
            <button
              type="button"
              className="btn btn-quiet"
              onClick={() => dispatch({ type: 'cancel' })}
            >
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" disabled={pending}>
              Save changes
            </button>
          </div>
        </form>
      ) : (
        <div className="comment-body">
          <Markdown text={c.body} />
        </div>
      )}
      {!ui.editing && (
        <div className="comment-actions">
          {onEdit && (
            <button
              type="button"
              className="comment-action"
              onClick={() => dispatch({ type: 'start-edit', body: c.body })}
              aria-label={`Edit comment ${c.id}`}
            >
              Edit
            </button>
          )}
          {c.hidden
            ? onUnhide && (
                <button
                  type="button"
                  className="comment-action"
                  disabled={pending}
                  onClick={() => onUnhide(c.id)}
                  aria-label={`Unhide comment ${c.id}`}
                >
                  Unhide
                </button>
              )
            : onHide && (
                <HideCommentDialog
                  commentId={c.id}
                  pending={pending}
                  onHide={(r) => onHide(c.id, r)}
                />
              )}
          {onDelete && (
            <ConfirmDialog
              label="Delete"
              title={`Delete comment #${c.id}?`}
              description="This permanently removes the comment. This action cannot be undone."
              confirmLabel="Delete comment"
              triggerClass="comment-action comment-action-danger"
              pending={pending}
              onConfirm={() => onDelete(c.id)}
            />
          )}
        </div>
      )}
    </li>
  )
}

export function CommentList({ comments, ...handlers }: { comments: Comment[] } & CommentHandlers) {
  if (comments.length === 0) {
    return <p className="muted">No comments yet.</p>
  }
  return (
    <ol className="comment-list" role="list">
      {comments.map((c) => (
        <CommentItem key={c.id} comment={c} {...handlers} />
      ))}
    </ol>
  )
}

// The issue article card. The comments section + form live next to it
// in the page component, mirroring the MPA's issue_detail.html.
export function IssueDetail({
  issue,
  ws,
  onSetStatus,
  onDelete,
  onCopyMarkdown,
  pending,
}: {
  issue: Issue
  // The workspace this issue is scoped to — its links stay under /w/{ws}.
  ws: string
  onSetStatus?: (status: 'open' | 'closed') => void
  onDelete?: () => void
  onCopyMarkdown?: () => Promise<string>
  pending?: boolean
}) {
  const next = issue.status === 'open' ? 'closed' : 'open'
  return (
    <article className="card issue-detail" id="issue-article">
      <header className="issue-detail-head">
        <div className="issue-title-row">
          <StatusBadge status={issue.status} />
          <h1>
            {issue.title} <span className="issue-num">#{issue.id}</span>
          </h1>
        </div>
        {issue.labels.length > 0 && (
          <div className="issue-labels">
            {issue.labels.map((l) => (
              <Link
                className="label-badge"
                style={labelStyle(l)}
                key={l.id}
                to="/w/$ws/issues"
                params={{ ws }}
                search={{ status: 'all', label: l.name }}
              >
                {l.name}
              </Link>
            ))}
          </div>
        )}
        <p className="issue-meta">
          Opened by {issue.author?.name ?? 'anonymous'}{' '}
          <time dateTime={issue.created_at}>{fmtTime(issue.created_at)}</time> · Last updated{' '}
          <time dateTime={issue.updated_at}>{fmtTime(issue.updated_at)}</time>
        </p>
      </header>

      {issue.body ? (
        <div className="issue-body">
          <Markdown text={issue.body} />
        </div>
      ) : (
        <div className="issue-body issue-body-empty">
          <em>No description provided.</em>
        </div>
      )}

      <div className="issue-actions">
        <Link
          className="btn"
          to="/w/$ws/issues/$issueId/edit"
          params={{ ws, issueId: String(issue.id) }}
        >
          Edit
        </Link>
        {onSetStatus && (
          <button
            type="button"
            className="btn"
            onClick={() => onSetStatus(next)}
            disabled={pending}
          >
            {next === 'closed' ? 'Close issue' : 'Reopen issue'}
          </button>
        )}
        {onCopyMarkdown && (
          <CopyButton label="Copy as markdown" getText={onCopyMarkdown} pending={pending} />
        )}
        {onDelete && (
          <ConfirmDialog
            label="Delete"
            title={`Delete issue #${issue.id}?`}
            description={`This permanently removes ${issue.title} and all of its comments. This action cannot be undone.`}
            confirmLabel="Delete issue"
            pending={pending}
            onConfirm={onDelete}
          />
        )}
      </div>
    </article>
  )
}
