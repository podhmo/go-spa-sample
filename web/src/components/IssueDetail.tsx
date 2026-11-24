import { Link } from '@tanstack/react-router'
import { useId, useReducer } from 'react'
import { HIDE_REASONS, type Comment, type Issue } from '../api'
import { ConfirmDialog } from './ConfirmDialog'
import { CopyButton } from './CopyButton'
import { LabelBadge, StatusBadge } from './IssueList'
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

export type CommentHandlers = {
  pending?: boolean
  onEdit?: (cid: number, body: string) => void
  onDelete?: (cid: number) => void
  onHide?: (cid: number, reason: string) => void
  onUnhide?: (cid: number) => void
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
  pending?: boolean
  onHide: (reason: string) => void
}) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const dialogId = `hide-${id}`
  return (
    <>
      <button
        type="button"
        {...({ commandfor: dialogId, command: 'show-modal' } as Record<string, string>)}
        disabled={pending}
      >
        hide
      </button>
      <dialog
        id={dialogId}
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
        <h1 id={`${dialogId}-title`}>Hide comment #{commentId}?</h1>
        <form method="dialog">
          <div className="field">
            <label htmlFor={`${dialogId}-reason`}>Reason</label>
            <select id={`${dialogId}-reason`} name="reason">
              {HIDE_REASONS.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
          </div>
          <button type="submit" value="cancel">
            cancel
          </button>{' '}
          <button type="submit" value="hide">
            hide comment
          </button>
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
    <li id={`comment-${c.id}`} data-hidden={c.hidden || undefined}>
      {c.hidden ? (
        <em>This comment was hidden{c.hidden_reason ? ` (${c.hidden_reason})` : ''}.</em>
      ) : ui.editing ? (
        <form
          aria-label={`edit comment ${c.id}`}
          onSubmit={(e) => {
            e.preventDefault()
            onEdit?.(c.id, ui.draft)
            dispatch({ type: 'cancel' })
          }}
        >
          <textarea
            value={ui.draft}
            onChange={(e) => dispatch({ type: 'draft', value: e.target.value })}
            rows={4}
            required
            aria-label="comment body"
          />{' '}
          <button type="submit" disabled={pending}>
            save
          </button>{' '}
          <button type="button" onClick={() => dispatch({ type: 'cancel' })}>
            cancel
          </button>
        </form>
      ) : (
        <Markdown text={c.body} />
      )}
      <small>
        {c.author ? c.author.name : 'anonymous'} — {c.created_at}
        {c.edited_at && (
          <>
            {' '}
            — <em>edited</em>
          </>
        )}
      </small>{' '}
      {!ui.editing && (
        <>
          {!c.hidden && onEdit && (
            <button type="button" onClick={() => dispatch({ type: 'start-edit', body: c.body })}>
              edit
            </button>
          )}{' '}
          {!c.hidden && onHide && (
            <HideCommentDialog commentId={c.id} pending={pending} onHide={(r) => onHide(c.id, r)} />
          )}{' '}
          {c.hidden && onUnhide && (
            <button type="button" disabled={pending} onClick={() => onUnhide(c.id)}>
              unhide
            </button>
          )}{' '}
          {!c.hidden && onDelete && (
            <ConfirmDialog
              label="delete"
              title={`Delete comment #${c.id}?`}
              confirmLabel="delete"
              pending={pending}
              onConfirm={() => onDelete(c.id)}
            />
          )}{' '}
          <CopyButton label="permalink" getText={permalink} pending={pending} />
        </>
      )}
    </li>
  )
}

export function CommentList({ comments, ...handlers }: { comments: Comment[] } & CommentHandlers) {
  if (comments.length === 0) {
    return <p>No comments yet.</p>
  }
  return (
    <ul>
      {comments.map((c) => (
        <CommentItem key={c.id} comment={c} {...handlers} />
      ))}
    </ul>
  )
}

export function IssueDetail({
  issue,
  comments,
  onSetStatus,
  onDelete,
  onCopyMarkdown,
  commentHandlers,
  pending,
}: {
  issue: Issue
  comments: Comment[]
  onSetStatus?: (status: 'open' | 'closed') => void
  onDelete?: () => void
  onCopyMarkdown?: () => Promise<string>
  commentHandlers?: CommentHandlers
  pending?: boolean
}) {
  const next = issue.status === 'open' ? 'closed' : 'open'
  return (
    <article>
      <header>
        <h2>
          #{issue.id} {issue.title} <StatusBadge status={issue.status} />
        </h2>
        <p>
          {issue.labels.map((l) => (
            <LabelBadge key={l.id} label={l} />
          ))}{' '}
          {issue.author ? `opened by ${issue.author.name}` : ''} — {issue.created_at}
        </p>
        {onSetStatus && (
          <button onClick={() => onSetStatus(next)} disabled={pending}>
            {pending ? '…' : `mark ${next}`}
          </button>
        )}{' '}
        <Link to="/issues/$issueId/edit" params={{ issueId: String(issue.id) }}>
          edit
        </Link>{' '}
        {onDelete && (
          <ConfirmDialog
            label="delete"
            title={`Delete issue #${issue.id}?`}
            confirmLabel="delete"
            pending={pending}
            onConfirm={onDelete}
          />
        )}{' '}
        {onCopyMarkdown && (
          <CopyButton label="copy as markdown" getText={onCopyMarkdown} pending={pending} />
        )}
      </header>
      <section>
        <Markdown text={issue.body} />
      </section>
      <section>
        <h3>Comments ({comments.length})</h3>
        <CommentList comments={comments} pending={pending} {...commentHandlers} />
      </section>
    </article>
  )
}
