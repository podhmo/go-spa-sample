import { Link } from '@tanstack/react-router'
import { useId, useReducer } from 'react'
import { useLeaveGuard } from '../../../../leave-guard'
import type { PRComment, PRCommit, PullMergeable, PullRequest } from '../../../../api'
import { ConfirmDialog } from '../../../../components/ConfirmDialog'
import dialogStyles from '../../../../components/ConfirmDialog.module.css'
import { CopyButton } from '../../../../components/CopyButton'
import { submitOnCtrlEnter } from '../../../../enter-submit'
import { fmt, useFmtTime, useMessages } from '../../../../i18n'
import { HideDialog } from '../../../../components/HideDialog'
import { Markdown } from '../../../../components/Markdown'
import { PullStatusBadge } from '../../-components/PullList'
import pullStyles from '../../-components/pulls.module.css'
import styles from './PullDetail.module.css'
import { cx } from '../../../../styles/cx'

const shortRef = (ref: string) => ref.replace(/^refs\/(heads|remotes|tags)\//, '')

// Merge-ability probe result, rendered like the MPA's merge box.
export function MergeableBadge({ mergeable }: { mergeable: PullMergeable }) {
  const m = useMessages()
  if (!mergeable) return null
  if (mergeable.status === 'clean') {
    return <span className={`badge ${pullStyles.badgePrOpen}`}>{m.pulls.mergeable}</span>
  }
  if (mergeable.status === 'conflicting') {
    return (
      <span className={`badge ${pullStyles.badgePrConflict}`} title={mergeable.files?.join(', ')}>
        {mergeable.files?.length
          ? fmt(m.pulls.conflicting, { n: mergeable.files.length })
          : m.pulls.conflictNoFiles}
      </span>
    )
  }
  return <span className="badge">{m.pulls.mergeUnknown}</span>
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
  const mm = useMessages()
  return (
    <>
      <button
        type="button"
        className={cx('btn btn-primary')}
        {...({ commandfor: dialogId, command: 'show-modal' } as Record<string, string>)}
        disabled={pending}
      >
        {mm.pulls.merge}
      </button>
      <dialog
        id={dialogId}
        className={dialogStyles.dialog}
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
        <h2 id={`${dialogId}-title`}>{fmt(mm.pulls.mergeTitle, { id: pullId })}</h2>
        <form method="dialog">
          <div className={cx('field')} role="radiogroup" aria-label={mm.pulls.mergeMethod}>
            {(['merge', 'squash', 'rebase'] as const).map((m) => (
              <label key={m} className={styles.mergeMethod}>
                <input
                  type="radio"
                  name="method"
                  value={m}
                  checked={method === m}
                  onChange={() => setMethod(m)}
                />{' '}
                {m === 'merge'
                  ? mm.pulls.mergeCommit
                  : m === 'squash'
                    ? mm.pulls.mergeSquash
                    : mm.pulls.mergeRebase}
              </label>
            ))}
          </div>
          {method !== 'rebase' && (
            <div className={cx('field')}>
              <label htmlFor={`${dialogId}-message`}>{mm.pulls.commitMessage}</label>
              <textarea
                id={`${dialogId}-message`}
                name="message"
                rows={3}
                placeholder={mm.pulls.commitMessagePh}
              />
            </div>
          )}
          <div className={cx('form-actions')}>
            <button type="submit" className={cx('btn')} value="cancel" formNoValidate autoFocus>
              {mm.copy.cancel}
            </button>
            <button type="submit" className={cx('btn btn-primary')} value="merge">
              {mm.pulls.mergeConfirm}
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
  const m = useMessages()
  const fmtTime = useFmtTime()
  return (
    <article className={cx('card issue-detail')} id="pull-article">
      <header className="issue-detail-head">
        <div className="issue-title-row">
          <PullStatusBadge status={p.status} />
          <h1>
            {p.title} <span className="issue-num">#{p.id}</span>
          </h1>
        </div>
        <p className="issue-meta">
          <code className={pullStyles.prRef}>{shortRef(p.head)}</code> →{' '}
          <code className={pullStyles.prRef}>{shortRef(p.base)}</code> ·{' '}
          {fmt(m.pulls.openedBy, { name: p.author?.name ?? m.comments.anonymous })}{' '}
          <time dateTime={p.created_at}>{fmtTime(p.created_at)}</time>
          {p.status === 'merged' && p.merge_method && (
            <> · {fmt(m.pulls.mergedVia, { method: p.merge_method })}</>
          )}
          {p.closed_at && p.status !== 'merged' && (
            <>
              {' '}
              · {m.pulls.closedAt} <time dateTime={p.closed_at}>{fmtTime(p.closed_at)}</time>
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
          <em>{m.pulls.noDescription}</em>
        </div>
      )}

      <div className="issue-actions">
        {p.status === 'open' && onMerge && mergeable?.status !== 'conflicting' && (
          <MergeDialog pullId={p.id} pending={pending} onMerge={onMerge} />
        )}
        {onSetStatus && p.status === 'open' && (
          <button
            type="button"
            className={cx('btn')}
            disabled={pending}
            onClick={() => onSetStatus('closed')}
          >
            {m.pulls.close}
          </button>
        )}
        {onSetStatus && p.status === 'closed' && (
          <button
            type="button"
            className={cx('btn')}
            disabled={pending}
            onClick={() => onSetStatus('open')}
          >
            {m.pulls.reopen}
          </button>
        )}
        <Link
          preload={false}
          className={cx('btn')}
          to="/pulls/$pullId/files"
          params={{ pullId: String(p.id) }}
        >
          {m.pulls.files}
        </Link>
        <Link className={cx('btn')} to="/pulls/$pullId/edit" params={{ pullId: String(p.id) }}>
          {m.comments.edit}
        </Link>
        {onCopyMarkdown && (
          <CopyButton label={m.copy.asMarkdown} getText={onCopyMarkdown} pending={pending} />
        )}
        {onDelete && (
          <ConfirmDialog
            label={m.pulls.delete}
            title={fmt(m.pulls.deleteTitle, { id: p.id })}
            description={fmt(m.pulls.deleteBody, { title: p.title })}
            confirmLabel={m.pulls.deleteConfirm}
            pending={pending}
            onConfirm={onDelete}
          />
        )}
      </div>
      {actionError && (
        <em className={`${cx('error-msg')} ${styles.pullActionError}`} role="alert">
          {actionError}
        </em>
      )}
    </article>
  )
}

export function PullCommitList({ commits }: { commits: PRCommit[] }) {
  const m = useMessages()
  const fmtTime = useFmtTime()
  if (commits.length === 0) return null
  return (
    <section className={styles.pullCommits}>
      <h2>{fmt(m.pulls.commits, { n: commits.length })}</h2>
      <ol className="comment-list" role="list">
        {commits.map((c) => (
          <li className={cx('card comment')} key={c.sha}>
            <header className="comment-meta">
              <code className={pullStyles.prSha}>{c.sha.slice(0, 10)}</code>{' '}
              <span className="comment-author">{c.subject}</span>{' '}
              <span className="muted">
                {fmt(m.pulls.by, { author: c.author })}{' '}
                <time dateTime={c.date}>{fmtTime(c.date)}</time>
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
  const [dirty, setDirty] = useReducer((_: boolean, v: boolean) => v, false)
  const [blank, setBlank] = useReducer((_: boolean, v: boolean) => v, false)
  const m = useMessages()
  useLeaveGuard(editing && dirty, m.form.unsavedChanges)
  // The textarea is uncontrolled, so every open/cancel/save restores
  // c.body in the DOM — the flags must be reset to match or a stale
  // dirty/blank survives into the next session (Save stuck disabled).
  const toggleEditing = () => {
    setDirty(false)
    setBlank(false)
    setEditing()
  }
  const fmtTime = useFmtTime()
  const id = `comment-${c.id}`
  return (
    <li className={`${cx('card comment')}${c.hidden ? ' comment-hidden' : ''}`} id={id}>
      <header className="comment-meta">
        <span className="comment-author">
          {fmt(m.comments.commented, { name: c.author?.name ?? m.comments.anonymous })}
        </span>{' '}
        {c.path && (
          <>
            {' '}
            {m.pulls.onFile}{' '}
            <code className={pullStyles.prRef}>
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
            · <span className="comment-edited">{m.comments.edited}</span>
          </>
        )}
      </header>
      {c.diff_snippet && !c.hidden && (
        <pre className={styles.diffSnippet} aria-label={m.pulls.diffContext}>
          <code>{c.diff_snippet}</code>
        </pre>
      )}
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
      ) : editing ? (
        <form
          aria-label={fmt(m.comments.editFormAria, { noun: m.comments.nounComment, id: c.id })}
          onSubmit={(e) => {
            e.preventDefault()
            const body = String(new FormData(e.currentTarget).get('body') ?? '')
            onEdit?.(c.id, body)
            toggleEditing()
          }}
        >
          <div className={cx('field')}>
            <label htmlFor={`edit-${c.id}`} className="visually-hidden">
              {m.comments.bodyLabel}
            </label>
            <textarea
              id={`edit-${c.id}`}
              name="body"
              defaultValue={c.body}
              onChange={(e) => {
                setDirty(e.target.value !== c.body)
                setBlank(e.target.value.trim() === '')
              }}
              onKeyDown={submitOnCtrlEnter}
              rows={4}
              required
              data-error={`edit-${c.id}-error`}
            />
            <span id={`edit-${c.id}-error`} className={cx('error-msg')}>
              {m.form.bodyRequired}
            </span>
          </div>
          <div className={cx('form-actions')}>
            <button type="button" className={cx('btn btn-quiet')} onClick={toggleEditing}>
              {m.copy.cancel}
            </button>
            <button type="submit" className={cx('btn btn-primary')} disabled={pending || blank}>
              {m.form.saveChanges}
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
              onClick={toggleEditing}
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
  const m = useMessages()
  if (comments.length === 0) {
    return <p className="muted">{m.comments.empty}</p>
  }
  return (
    <ol className="comment-list" role="list">
      {comments.map((c) => (
        <PRCommentItem key={c.id} comment={c} {...handlers} />
      ))}
    </ol>
  )
}
