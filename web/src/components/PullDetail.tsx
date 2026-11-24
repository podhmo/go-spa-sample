import { Link } from '@tanstack/react-router'
import { useId, useReducer } from 'react'
import type { PRComment, PRCommit, PullMergeable, PullRequest } from '../api'
import { ConfirmDialog } from './ConfirmDialog'
import { CopyButton } from './CopyButton'
import { submitOnCtrlEnter } from '../enter-submit'
import { fmtTime } from '../format'
import { HideDialog } from './DiscussionDetail'
import { Markdown } from './Markdown'
import { PullStatusBadge } from './PullList'

const shortRef = (ref: string) => ref.replace(/^refs\/(heads|remotes|tags)\//, '')

// Merge-ability probe result, rendered like the MPA's merge box.
export function MergeableBadge({ mergeable }: { mergeable: PullMergeable }) {
  if (!mergeable) return null
  if (mergeable.status === 'clean') {
    return <span className="badge badge-pr-open">mergeable</span>
  }
  if (mergeable.status === 'conflicting') {
    return (
      <span className="badge badge-pr-conflict" title={mergeable.files?.join(', ')}>
        conflicts{mergeable.files?.length ? ` in ${mergeable.files.length} file(s)` : ''}
      </span>
    )
  }
  return <span className="badge">mergeability unknown</span>
}

// The merge dialog: method radio (merge/squash/rebase) + optional
// message, matching POST /api/v1/pulls/{id}/merge.
export function MergeDialog({
  pullId,
  pending,
  onMerge,
}: {
  pullId: number
  pending?: boolean | undefined
  onMerge: (method: string, message: string) => void
}) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const dialogId = `merge-${id}`
  // Rebase replays each commit's own message — a custom message would
  // be silently discarded, so the field hides for that method.
  const [method, setMethod] = useReducer((_: string, v: string) => v, 'merge')
  return (
    <>
      <button
        type="button"
        className="btn btn-primary"
        {...({ commandfor: dialogId, command: 'show-modal' } as Record<string, string>)}
        disabled={pending}
      >
        Merge
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
          if (d.returnValue === 'merge') {
            const form = d.querySelector('form')
            const fd = new FormData(form ?? undefined)
            const method = String(fd.get('method') ?? 'merge')
            const message = String(fd.get('message') ?? '')
            onMerge(method, message)
          }
          d.returnValue = ''
        }}
      >
        <h2 id={`${dialogId}-title`}>Merge pull request #{pullId}?</h2>
        <form method="dialog">
          <div className="field" role="radiogroup" aria-label="Merge method">
            {(['merge', 'squash', 'rebase'] as const).map((m) => (
              <label key={m} className="merge-method">
                <input
                  type="radio"
                  name="method"
                  value={m}
                  checked={method === m}
                  onChange={() => setMethod(m)}
                />{' '}
                {m === 'merge'
                  ? 'Create a merge commit'
                  : m === 'squash'
                    ? 'Squash and merge'
                    : 'Rebase and merge'}
              </label>
            ))}
          </div>
          {method !== 'rebase' && (
            <div className="field">
              <label htmlFor={`${dialogId}-message`}>Commit message (optional)</label>
              <textarea
                id={`${dialogId}-message`}
                name="message"
                rows={3}
                placeholder="Leave empty for the default message"
              />
            </div>
          )}
          <div className="form-actions">
            <button type="submit" className="btn" value="cancel" formNoValidate autoFocus>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" value="merge">
              Merge pull request
            </button>
          </div>
        </form>
      </dialog>
    </>
  )
}

// The PR article card: status, refs, mergeability, meta, actions.
export function PullDetail({
  pull,
  mergeable,
  actionError,
  onDelete,
  onCopyMarkdown,
  onSetStatus,
  onMerge,
  pending,
}: {
  pull: PullRequest
  mergeable?: PullMergeable | undefined
  actionError?: string | null | undefined
  onDelete?: () => void
  onCopyMarkdown?: () => Promise<string>
  onSetStatus?: (status: 'open' | 'closed') => void
  onMerge?: (method: string, message: string) => void
  pending?: boolean
}) {
  const p = pull
  return (
    <article className="card issue-detail" id="pull-article">
      <header className="issue-detail-head">
        <div className="issue-title-row">
          <PullStatusBadge status={p.status} />
          <h1>
            {p.title} <span className="issue-num">#{p.id}</span>
          </h1>
        </div>
        <p className="issue-meta">
          <code className="pr-ref">{shortRef(p.head)}</code> →{' '}
          <code className="pr-ref">{shortRef(p.base)}</code> · Opened by{' '}
          {p.author?.name ?? 'anonymous'}{' '}
          <time dateTime={p.created_at}>{fmtTime(p.created_at)}</time>
          {p.status === 'merged' && p.merge_method && (
            <>
              {' '}
              · merged via <code>{p.merge_method}</code>
            </>
          )}
          {p.closed_at && p.status !== 'merged' && (
            <>
              {' '}
              · closed <time dateTime={p.closed_at}>{fmtTime(p.closed_at)}</time>
            </>
          )}
        </p>
        {p.status === 'open' && <MergeableBadge mergeable={mergeable} />}
      </header>

      {p.body ? (
        <div className="issue-body">
          <Markdown text={p.body} />
        </div>
      ) : (
        <div className="issue-body issue-body-empty">
          <em>No description provided.</em>
        </div>
      )}

      <div className="issue-actions">
        {p.status === 'open' && onMerge && (
          <MergeDialog pullId={p.id} pending={pending} onMerge={onMerge} />
        )}
        {onSetStatus && p.status === 'open' && (
          <button
            type="button"
            className="btn"
            disabled={pending}
            onClick={() => onSetStatus('closed')}
          >
            Close
          </button>
        )}
        {onSetStatus && p.status === 'closed' && (
          <button
            type="button"
            className="btn"
            disabled={pending}
            onClick={() => onSetStatus('open')}
          >
            Reopen
          </button>
        )}
        <Link className="btn" to="/pulls/$pullId/files" params={{ pullId: String(p.id) }}>
          Files
        </Link>
        <Link className="btn" to="/pulls/$pullId/edit" params={{ pullId: String(p.id) }}>
          Edit
        </Link>
        {onCopyMarkdown && (
          <CopyButton label="Copy as markdown" getText={onCopyMarkdown} pending={pending} />
        )}
        {onDelete && (
          <ConfirmDialog
            label="Delete"
            title={`Delete pull request #${p.id}?`}
            description={`This permanently removes ${p.title} and all of its comments. This action cannot be undone.`}
            confirmLabel="Delete pull request"
            pending={pending}
            onConfirm={onDelete}
          />
        )}
      </div>
      {actionError && (
        <em className="pull-action-error" role="alert">
          {actionError}
        </em>
      )}
    </article>
  )
}

export function PullCommitList({ commits }: { commits: PRCommit[] }) {
  if (commits.length === 0) return null
  return (
    <section className="pull-commits">
      <h2>Commits ({commits.length})</h2>
      <ol className="comment-list" role="list">
        {commits.map((c) => (
          <li className="card comment" key={c.sha}>
            <header className="comment-meta">
              <code className="pr-sha">{c.sha.slice(0, 10)}</code>{' '}
              <span className="comment-author">{c.subject}</span>{' '}
              <span className="muted">
                by {c.author} <time dateTime={c.date}>{fmtTime(c.date)}</time>
              </span>
            </header>
          </li>
        ))}
      </ol>
    </section>
  )
}

// One timeline entry: a conversation comment or a review comment
// (anchored to a diff line, carrying its snippet).
export function PRCommentItem({
  comment: c,
  pending,
  onEdit,
  onDelete,
  onHide,
  onUnhide,
}: {
  comment: PRComment
  pending?: boolean | undefined
  onEdit?: (cid: number, body: string) => void
  onDelete?: (cid: number) => void
  onHide?: (cid: number, reason: string) => void
  onUnhide?: (cid: number) => void
}) {
  const [editing, setEditing] = useReducer((s: boolean) => !s, false)
  const id = `comment-${c.id}`
  return (
    <li className={`card comment${c.hidden ? ' comment-hidden' : ''}`} id={id}>
      <header className="comment-meta">
        <span className="comment-author">{c.author?.name ?? 'anonymous'} commented</span>{' '}
        {c.path && (
          <>
            on{' '}
            <code className="pr-ref">
              {c.path}:{c.line}
            </code>{' '}
          </>
        )}
        <a className="comment-permalink" href={`#${id}`}>
          <time dateTime={c.created_at}>{fmtTime(c.created_at)}</time>
        </a>
        {c.edited_at && (
          <>
            {' '}
            · <span className="comment-edited">edited</span>
          </>
        )}
      </header>
      {c.diff_snippet && !c.hidden && (
        <pre className="diff-snippet" aria-label="Diff context">
          <code>{c.diff_snippet}</code>
        </pre>
      )}
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
      ) : editing ? (
        <form
          aria-label={`edit comment ${c.id}`}
          onSubmit={(e) => {
            e.preventDefault()
            const body = String(new FormData(e.currentTarget).get('body') ?? '')
            onEdit?.(c.id, body)
            setEditing()
          }}
        >
          <div className="field">
            <label htmlFor={`edit-${c.id}`} className="visually-hidden">
              body
            </label>
            <textarea
              id={`edit-${c.id}`}
              name="body"
              defaultValue={c.body}
              onKeyDown={submitOnCtrlEnter}
              rows={4}
              required
              data-error={`edit-${c.id}-error`}
            />
            <span id={`edit-${c.id}-error`} className="error-msg">
              Body is required.
            </span>
          </div>
          <div className="form-actions">
            <button type="button" className="btn btn-quiet" onClick={() => setEditing()}>
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
      {!editing && (
        <div className="comment-actions">
          {onEdit && !c.hidden && (
            <button
              type="button"
              className="comment-action"
              onClick={() => setEditing()}
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
                <HideDialog
                  noun="comment"
                  itemId={c.id}
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

export function PRCommentList({
  comments,
  ...handlers
}: {
  comments: PRComment[]
  pending?: boolean | undefined
  onEdit?: (cid: number, body: string) => void
  onDelete?: (cid: number) => void
  onHide?: (cid: number, reason: string) => void
  onUnhide?: (cid: number) => void
}) {
  if (comments.length === 0) {
    return <p className="muted">No comments yet.</p>
  }
  return (
    <ol className="comment-list" role="list">
      {comments.map((c) => (
        <PRCommentItem key={c.id} comment={c} {...handlers} />
      ))}
    </ol>
  )
}
