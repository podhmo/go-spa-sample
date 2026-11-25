import { Link } from '@tanstack/react-router'
import { useReducer } from 'react'
import { useLeaveGuard } from '../../../../../../leave-guard'
import type { Comment, Issue } from '../../../../../../api'
import { ConfirmDialog } from '../../../../../../components/ConfirmDialog'
import { CopyButton } from '../../../../../../components/CopyButton'
import { HideDialog } from '../../../../../../components/HideDialog'
import { submitOnCtrlEnter } from '../../../../../../enter-submit'
import { labelStyle } from '../../../../../../format'
import { fmt, useFmtTime, useMessages } from '../../../../../../i18n'
import { StatusBadge } from '../../-components/IssueList'
import { Markdown } from '../../../../../../components/Markdown'
import { cx } from '../../../../../../styles/cx'

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
  editError?: { cid: number; message: string } | null | undefined
  onEditErrorClear?: (() => void) | undefined
  onEdit?: ((cid: number, body: string) => void | boolean | Promise<void | boolean>) | undefined
  onDelete?: ((cid: number) => void) | undefined
  onHide?: ((cid: number, reason: string) => void) | undefined
  onUnhide?: ((cid: number) => void) | undefined
}

function CommentItem({
  comment: c,
  pending,
  editError,
  onEditErrorClear,
  onEdit,
  onDelete,
  onHide,
  onUnhide,
}: { comment: Comment } & CommentHandlers) {
  const [ui, dispatch] = useReducer(commentUIReducer, { editing: false, draft: '' })
  const fieldError = editError?.cid === c.id ? editError.message : null
  const m = useMessages()
  useLeaveGuard(ui.editing && ui.draft !== c.body, m.form.unsavedChanges)
  const fmtTime = useFmtTime()
  const permalink = () => `${location.origin}${location.pathname}#comment-${c.id}`
  return (
    <li
      className={`${cx('card comment')}${c.hidden ? ' comment-hidden' : ''}`}
      id={`comment-${c.id}`}
    >
      <header className="comment-meta">
        <span className="comment-author">
          {fmt(m.comments.commented, { name: c.author?.name ?? m.comments.anonymous })}
        </span>{' '}
        <a className="comment-permalink" href={`#comment-${c.id}`}>
          <time dateTime={c.created_at}>{fmtTime(c.created_at)}</time>
        </a>
        {c.edited_at && (
          <>
            {' '}
            · <span className="comment-edited">{m.comments.edited}</span>
          </>
        )}
        <CopyButton
          className="copy-btn"
          label={m.comments.copyLink}
          getText={permalink}
          pending={pending}
        />
      </header>
      {c.hidden ? (
        <>
          <p className="comment-hidden-note">
            {fmt(m.comments.markedAs, {
              noun: m.comments.nounComment,
              reason: c.hidden_reason ?? 'hidden',
            })}
          </p>
          <details className="comment-hidden-body">
            <summary>{fmt(m.comments.show, { noun: m.comments.nounComment })}</summary>
            <div className="comment-body">
              <Markdown text={c.body} />
            </div>
          </details>
        </>
      ) : ui.editing ? (
        <form
          aria-label={fmt(m.comments.editFormAria, { noun: m.comments.nounComment, id: c.id })}
          onSubmit={(e) => {
            e.preventDefault()
            void (async () => {
              const ok = await onEdit?.(c.id, ui.draft)
              if (ok !== false) dispatch({ type: 'cancel' })
            })()
          }}
        >
          <div className={cx('field')}>
            <label htmlFor={`comment-edit-${c.id}`} className="visually-hidden">
              {m.comments.commentBodyLabel}
            </label>
            <textarea
              id={`comment-edit-${c.id}`}
              value={ui.draft}
              onChange={(e) => dispatch({ type: 'draft', value: e.target.value })}
              onKeyDown={submitOnCtrlEnter}
              rows={4}
              required
              disabled={pending}
              data-error={`comment-edit-${c.id}-error`}
              aria-invalid={fieldError ? 'true' : 'false'}
              aria-describedby={fieldError ? `comment-edit-${c.id}-api-error` : undefined}
              aria-label={m.comments.commentBodyLabel}
            />
            <span id={`comment-edit-${c.id}-error`} className={cx('error-msg')}>
              {m.form.bodyRequired}
            </span>
            {fieldError && (
              <em className={cx('error-msg')} id={`comment-edit-${c.id}-api-error`} role="alert">
                {fieldError}
              </em>
            )}
          </div>
          <div className={cx('form-actions')}>
            <button
              type="button"
              className={cx('btn btn-quiet')}
              onClick={() => {
                if (fieldError) onEditErrorClear?.()
                dispatch({ type: 'cancel' })
              }}
            >
              {m.copy.cancel}
            </button>
            <button
              type="submit"
              className={cx('btn btn-primary')}
              disabled={pending || !ui.draft.trim()}
            >
              {m.form.saveChanges}
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
              onClick={() => {
                if (fieldError) onEditErrorClear?.()
                dispatch({ type: 'start-edit', body: c.body })
              }}
              aria-label={fmt(m.comments.ariaEdit, { noun: m.comments.nounComment, id: c.id })}
            >
              {m.comments.edit}
            </button>
          )}
          {c.hidden
            ? onUnhide && (
                <button
                  type="button"
                  className="comment-action"
                  disabled={pending}
                  onClick={() => onUnhide(c.id)}
                  aria-label={fmt(m.comments.ariaUnhide, {
                    noun: m.comments.nounComment,
                    id: c.id,
                  })}
                >
                  {m.comments.unhide}
                </button>
              )
            : onHide && (
                <HideDialog
                  noun={m.comments.nounComment}
                  itemId={c.id}
                  pending={pending}
                  onHide={(r) => onHide(c.id, r)}
                />
              )}
          {onDelete && (
            <ConfirmDialog
              label={m.comments.delete}
              title={fmt(m.comments.deleteTitle, { noun: m.comments.nounComment, id: c.id })}
              description={m.comments.deleteCommentBody}
              confirmLabel={fmt(m.comments.deleteConfirm, { noun: m.comments.nounComment })}
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
  const m = useMessages()
  if (comments.length === 0) {
    return <p className="muted">{m.comments.empty}</p>
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
  const m = useMessages()
  const fmtTime = useFmtTime()
  const next = issue.status === 'open' ? 'closed' : 'open'
  return (
    <article className={cx('card issue-detail')} id="issue-article">
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
          {fmt(m.issues.openedBy, { name: issue.author?.name ?? m.comments.anonymous })}{' '}
          <time dateTime={issue.created_at}>{fmtTime(issue.created_at)}</time> ·{' '}
          {m.issues.lastUpdated}{' '}
          <time dateTime={issue.updated_at}>{fmtTime(issue.updated_at)}</time>
        </p>
      </header>

      {issue.body ? (
        <div className="issue-body">
          <Markdown text={issue.body} />
        </div>
      ) : (
        <div className="issue-body issue-body-empty">
          <em>{m.issues.noDescription}</em>
        </div>
      )}

      <div className="issue-actions">
        <Link
          className={cx('btn')}
          to="/w/$ws/issues/$issueId/edit"
          params={{ ws, issueId: String(issue.id) }}
        >
          {m.comments.edit}
        </Link>
        {onSetStatus && (
          <button
            type="button"
            className={cx('btn')}
            onClick={() => onSetStatus(next)}
            disabled={pending}
          >
            {next === 'closed' ? m.issues.close : m.issues.reopen}
          </button>
        )}
        {onCopyMarkdown && (
          <CopyButton label={m.copy.asMarkdown} getText={onCopyMarkdown} pending={pending} />
        )}
        {onDelete && (
          <ConfirmDialog
            label={m.issues.delete}
            title={fmt(m.issues.deleteTitle, { id: issue.id })}
            description={fmt(m.issues.deleteBody, { title: issue.title })}
            confirmLabel={m.issues.deleteConfirm}
            pending={pending}
            onConfirm={onDelete}
          />
        )}
      </div>
    </article>
  )
}
