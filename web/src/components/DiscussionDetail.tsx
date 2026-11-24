import { Link } from '@tanstack/react-router'
import { useId, useReducer } from 'react'
import { HIDE_REASONS, type Discussion, type DiscussionReply, type DiscussionThread } from '../api'
import { ConfirmDialog } from './ConfirmDialog'
import { CopyButton } from './CopyButton'
import { submitOnCtrlEnter } from '../enter-submit'
import { fmtTime, labelStyle } from '../format'
import { AnsweredBadge, CategoryBadge } from './DiscussionList'
import { Markdown } from './Markdown'

type EditUI = { editing: boolean; replying: boolean; draft: string }
type EditAction =
  | { type: 'start-edit'; body: string }
  | { type: 'cancel' }
  | { type: 'draft'; value: string }
  | { type: 'reply'; replying: boolean }

const editReducer = (s: EditUI, a: EditAction): EditUI => {
  switch (a.type) {
    case 'start-edit':
      return { ...s, editing: true, draft: a.body }
    case 'cancel':
      return { ...s, editing: false, replying: false, draft: '' }
    case 'draft':
      return { ...s, draft: a.value }
    case 'reply':
      // The reply form shares `draft` with the edit forms — always open
      // it empty so a previous edit/reply body is never reposted.
      return { ...s, replying: a.replying, draft: '' }
  }
}

// Optional members are spread into item props, so callers may pass
// explicit undefined (exactOptionalPropertyTypes).
export type ThreadHandlers = {
  pending?: boolean | undefined
  answerable?: boolean | undefined
  onEdit?: ((tid: number, body: string) => void) | undefined
  onDelete?: ((tid: number) => void) | undefined
  onHide?: ((tid: number, reason: string) => void) | undefined
  onUnhide?: ((tid: number) => void) | undefined
  onSetAnswer?: ((tid: number, answer: boolean) => void) | undefined
  onReply?: ((tid: number, body: string) => void) | undefined
}

export type ReplyHandlers = {
  pending?: boolean | undefined
  onEdit?: ((tid: number, cid: number, body: string) => void) | undefined
  onDelete?: ((tid: number, cid: number) => void) | undefined
  onHide?: ((tid: number, cid: number, reason: string) => void) | undefined
  onUnhide?: ((tid: number, cid: number) => void) | undefined
}

// Hide reason picker: same native <dialog> + <form method="dialog">
// pattern as the comment one, but generalized over the noun.
export function HideDialog({
  noun,
  itemId,
  pending,
  onHide,
}: {
  noun: string
  itemId: number
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
        aria-label={`Hide ${noun} ${itemId}`}
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
        <h2 id={`${dialogId}-title`}>
          Hide {noun} #{itemId}?
        </h2>
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
              Hide {noun}
            </button>
          </div>
        </form>
      </dialog>
    </>
  )
}

// The shared edit-in-place textarea for threads and replies.
function InlineEditForm({
  label,
  initial,
  pending,
  onSave,
  onCancel,
}: {
  label: string
  initial: string
  pending?: boolean | undefined
  onSave: (body: string) => void
  onCancel: () => void
}) {
  const [draft, setDraft] = useReducer((_: string, v: string) => v, initial)
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  return (
    <form
      aria-label={label}
      onSubmit={(e) => {
        e.preventDefault()
        onSave(draft)
        onCancel()
      }}
    >
      <div className="field">
        <label htmlFor={`edit-${id}`} className="visually-hidden">
          body
        </label>
        <textarea
          id={`edit-${id}`}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={submitOnCtrlEnter}
          rows={4}
          required
          aria-label="body"
          data-error={`edit-${id}-error`}
        />
        <span id={`edit-${id}-error`} className="error-msg">
          Body is required.
        </span>
      </div>
      <div className="form-actions">
        <button type="button" className="btn btn-quiet" onClick={onCancel}>
          Cancel
        </button>
        <button type="submit" className="btn btn-primary" disabled={pending}>
          Save changes
        </button>
      </div>
    </form>
  )
}

// The shared hidden/visible body: a moderation note plus the body tucked
// behind <details> (the MPA's collapsed-hidden idiom).
function ModeratedBody({
  body,
  hidden,
  hiddenReason,
  noun,
}: {
  body: string
  hidden: boolean
  hiddenReason?: string | undefined
  noun: string
}) {
  if (!hidden) {
    return (
      <div className="comment-body">
        <Markdown text={body} />
      </div>
    )
  }
  return (
    <>
      <p className="comment-hidden-note">
        This {noun} was marked as {hiddenReason ?? 'hidden'}.
      </p>
      <details className="comment-hidden-body">
        <summary>show {noun}</summary>
        <div className="comment-body">
          <Markdown text={body} />
        </div>
      </details>
    </>
  )
}

function ReplyItem({
  reply: c,
  threadId,
  pending,
  onEdit,
  onDelete,
  onHide,
  onUnhide,
}: { reply: DiscussionReply; threadId: number } & ReplyHandlers) {
  const [ui, dispatch] = useReducer(editReducer, { editing: false, replying: false, draft: '' })
  return (
    <li
      className={`card comment comment-nested${c.hidden ? ' comment-hidden' : ''}`}
      id={`comment-${c.id}`}
    >
      <header className="comment-meta">
        <span className="comment-author">{c.author?.name ?? 'anonymous'} replied</span>{' '}
        <a className="comment-permalink" href={`#comment-${c.id}`}>
          <time dateTime={c.created_at}>{fmtTime(c.created_at)}</time>
        </a>
        {c.edited_at && (
          <>
            {' '}
            · <span className="comment-edited">edited</span>
          </>
        )}
      </header>
      {c.hidden ? (
        <ModeratedBody body={c.body} hidden noun="reply" hiddenReason={c.hidden_reason} />
      ) : ui.editing ? (
        <InlineEditForm
          label={`edit reply ${c.id}`}
          initial={c.body}
          pending={pending}
          onSave={(body) => onEdit?.(threadId, c.id, body)}
          onCancel={() => dispatch({ type: 'cancel' })}
        />
      ) : (
        <div className="comment-body">
          <Markdown text={c.body} />
        </div>
      )}
      {!ui.editing && (
        <div className="comment-actions">
          {onEdit && !c.hidden && (
            <button
              type="button"
              className="comment-action"
              onClick={() => dispatch({ type: 'start-edit', body: c.body })}
              aria-label={`Edit reply ${c.id}`}
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
                  onClick={() => onUnhide(threadId, c.id)}
                  aria-label={`Unhide reply ${c.id}`}
                >
                  Unhide
                </button>
              )
            : onHide && (
                <HideDialog
                  noun="reply"
                  itemId={c.id}
                  pending={pending}
                  onHide={(r) => onHide(threadId, c.id, r)}
                />
              )}
          {onDelete && (
            <ConfirmDialog
              label="Delete"
              title={`Delete reply #${c.id}?`}
              description="This permanently removes the reply. This action cannot be undone."
              confirmLabel="Delete reply"
              triggerClass="comment-action comment-action-danger"
              pending={pending}
              onConfirm={() => onDelete(threadId, c.id)}
            />
          )}
        </div>
      )}
    </li>
  )
}

function ThreadItem({
  thread: t,
  pending,
  answerable,
  onEdit,
  onDelete,
  onHide,
  onUnhide,
  onSetAnswer,
  onReply,
  replyHandlers,
}: { thread: DiscussionThread } & ThreadHandlers & {
    replyHandlers: ReplyHandlers
  }) {
  const [ui, dispatch] = useReducer(editReducer, { editing: false, replying: false, draft: '' })
  const permalink = () => `${location.origin}${location.pathname}#thread-${t.id}`
  return (
    <li
      className={`card comment${t.is_answer ? ' comment-answer' : ''}${t.hidden ? ' comment-hidden' : ''}`}
      id={`thread-${t.id}`}
    >
      <header className="comment-meta">
        {t.is_answer && <AnsweredBadge />}{' '}
        <span className="comment-author">{t.author?.name ?? 'anonymous'} commented</span>{' '}
        <a className="comment-permalink" href={`#thread-${t.id}`}>
          <time dateTime={t.created_at}>{fmtTime(t.created_at)}</time>
        </a>
        {t.edited_at && (
          <>
            {' '}
            · <span className="comment-edited">edited</span>
          </>
        )}
        <CopyButton className="copy-btn" label="copy link" getText={permalink} pending={pending} />
      </header>
      {t.hidden ? (
        <ModeratedBody body={t.body} hidden noun="thread" hiddenReason={t.hidden_reason} />
      ) : ui.editing ? (
        <InlineEditForm
          label={`edit thread ${t.id}`}
          initial={t.body}
          pending={pending}
          onSave={(body) => onEdit?.(t.id, body)}
          onCancel={() => dispatch({ type: 'cancel' })}
        />
      ) : (
        <div className="comment-body">
          <Markdown text={t.body} />
        </div>
      )}
      {!ui.editing && (
        <div className="comment-actions">
          {onEdit && !t.hidden && (
            <button
              type="button"
              className="comment-action"
              onClick={() => dispatch({ type: 'start-edit', body: t.body })}
              aria-label={`Edit thread ${t.id}`}
            >
              Edit
            </button>
          )}
          {answerable &&
            onSetAnswer &&
            (t.is_answer ? (
              <button
                type="button"
                className="comment-action"
                disabled={pending}
                onClick={() => onSetAnswer(t.id, false)}
              >
                Unmark answer
              </button>
            ) : (
              !t.hidden && (
                <button
                  type="button"
                  className="comment-action"
                  disabled={pending}
                  onClick={() => onSetAnswer(t.id, true)}
                >
                  Mark as answer
                </button>
              )
            ))}
          {onReply && !t.hidden && (
            <button
              type="button"
              className="comment-action"
              onClick={() => dispatch({ type: 'reply', replying: !ui.replying })}
              aria-expanded={ui.replying}
            >
              Reply
            </button>
          )}
          {t.hidden
            ? onUnhide && (
                <button
                  type="button"
                  className="comment-action"
                  disabled={pending}
                  onClick={() => onUnhide(t.id)}
                  aria-label={`Unhide thread ${t.id}`}
                >
                  Unhide
                </button>
              )
            : onHide && (
                <HideDialog
                  noun="thread"
                  itemId={t.id}
                  pending={pending}
                  onHide={(r) => onHide(t.id, r)}
                />
              )}
          {onDelete && (
            <ConfirmDialog
              label="Delete"
              title={`Delete thread #${t.id}?`}
              description="This permanently removes the thread and its replies. This action cannot be undone."
              confirmLabel="Delete thread"
              triggerClass="comment-action comment-action-danger"
              pending={pending}
              onConfirm={() => onDelete(t.id)}
            />
          )}
        </div>
      )}
      {ui.replying && (
        <form
          className="comment-reply-form"
          aria-label={`reply to thread ${t.id}`}
          onSubmit={(e) => {
            e.preventDefault()
            onReply?.(t.id, ui.draft)
            dispatch({ type: 'cancel' })
          }}
        >
          <div className="field">
            <label htmlFor={`reply-${t.id}`} className="visually-hidden">
              reply
            </label>
            <textarea
              id={`reply-${t.id}`}
              value={ui.draft}
              onChange={(e) => dispatch({ type: 'draft', value: e.target.value })}
              onKeyDown={submitOnCtrlEnter}
              rows={3}
              required
              placeholder="Reply…"
              data-error={`reply-${t.id}-error`}
            />
            <span id={`reply-${t.id}-error`} className="error-msg">
              Reply is required.
            </span>
          </div>
          <div className="form-actions">
            <button type="submit" className="btn" disabled={pending}>
              Reply
            </button>
          </div>
        </form>
      )}
      {t.comments.length > 0 && (
        <ol className="comment-list comment-list-nested" role="list">
          {t.comments.map((c) => (
            <ReplyItem key={c.id} reply={c} threadId={t.id} {...replyHandlers} />
          ))}
        </ol>
      )}
    </li>
  )
}

export function ThreadList({
  threads,
  replyHandlers,
  ...handlers
}: { threads: DiscussionThread[] } & ThreadHandlers & {
    replyHandlers: ReplyHandlers
  }) {
  if (threads.length === 0) {
    return <p className="muted">No comments yet.</p>
  }
  return (
    <ol className="comment-list" role="list">
      {threads.map((t) => (
        <ThreadItem key={t.id} thread={t} replyHandlers={replyHandlers} {...handlers} />
      ))}
    </ol>
  )
}

// The discussion article card. The threads section + comment form live
// next to it in the page component, like the issue page.
export function DiscussionDetail({
  discussion,
  ws,
  onDelete,
  onCopyMarkdown,
  pending,
}: {
  discussion: Discussion
  // The workspace this discussion is scoped to — links stay under /w/{ws}.
  ws: string
  onDelete?: () => void
  onCopyMarkdown?: () => Promise<string>
  pending?: boolean
}) {
  const d = discussion
  return (
    <article className="card issue-detail" id="discussion-article">
      <header className="issue-detail-head">
        <div className="issue-title-row">
          <CategoryBadge name={d.category.name} />
          {d.answered && <AnsweredBadge />}
          <h1>
            {d.title} <span className="issue-num">#{d.id}</span>
          </h1>
        </div>
        {d.labels.length > 0 && (
          <div className="issue-labels">
            {d.labels.map((l) => (
              <span className="label-badge" style={labelStyle(l)} key={l.id}>
                {l.name}
              </span>
            ))}
          </div>
        )}
        <p className="issue-meta">
          Opened by {d.author?.name ?? 'anonymous'}{' '}
          <time dateTime={d.created_at}>{fmtTime(d.created_at)}</time> · Last updated{' '}
          <time dateTime={d.updated_at}>{fmtTime(d.updated_at)}</time>
        </p>
      </header>

      {d.body ? (
        <div className="issue-body">
          <Markdown text={d.body} />
        </div>
      ) : (
        <div className="issue-body issue-body-empty">
          <em>No description provided.</em>
        </div>
      )}

      <div className="issue-actions">
        <Link
          className="btn"
          to="/w/$ws/discussions/$discussionId/edit"
          params={{ ws, discussionId: String(d.id) }}
        >
          Edit
        </Link>
        {onCopyMarkdown && (
          <CopyButton label="Copy as markdown" getText={onCopyMarkdown} pending={pending} />
        )}
        {onDelete && (
          <ConfirmDialog
            label="Delete"
            title={`Delete discussion #${d.id}?`}
            description={`This permanently removes ${d.title} and all of its comments. This action cannot be undone.`}
            confirmLabel="Delete discussion"
            pending={pending}
            onConfirm={onDelete}
          />
        )}
      </div>
    </article>
  )
}
