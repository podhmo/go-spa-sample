import { Link } from '@tanstack/react-router'
import { useId, useReducer } from 'react'
import { HIDE_REASONS, type Discussion, type DiscussionReply, type DiscussionThread } from '../api'
import { ConfirmDialog } from './ConfirmDialog'
import { CopyButton } from './CopyButton'
import { submitOnCtrlEnter } from '../enter-submit'
import { labelStyle } from '../format'
import { fmt, useFmtTime, useMessages } from '../i18n'
import { AnsweredBadge, CategoryBadge } from './DiscussionList'
import { Markdown } from './Markdown'

type EditUI = { editing: boolean; replying: boolean; draft: string }
type EditAction =
  | { type: 'start-edit'; body: string }
  | { type: 'cancel' }
  | { type: 'submitted' }
  | { type: 'draft'; value: string }
  | { type: 'reply'; replying: boolean }

const editReducer = (s: EditUI, a: EditAction): EditUI => {
  switch (a.type) {
    case 'start-edit':
      return { ...s, editing: true, draft: a.body }
    case 'cancel':
      // Close but keep the draft: a cancelled reply restores on reopen.
      return { ...s, editing: false, replying: false }
    case 'submitted':
      return { ...s, editing: false, replying: false, draft: '' }
    case 'draft':
      return { ...s, draft: a.value }
    case 'reply':
      // Pure toggle: collapsing must not throw the draft away, and
      // reopening restores it. A successful submit clears via 'submitted',
      // and 'start-edit' overwrites the shared draft with the edit body.
      return { ...s, replying: a.replying }
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
  onReply?: ((tid: number, body: string) => Promise<boolean>) | undefined
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
  const m = useMessages()
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const dialogId = `hide-${id}`
  return (
    <>
      <button
        type="button"
        className="comment-action"
        {...({ commandfor: dialogId, command: 'show-modal' } as Record<string, string>)}
        disabled={pending}
        aria-label={fmt(m.comments.ariaHide, { noun, id: itemId })}
      >
        {m.comments.hide}
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
        <h2 id={`${dialogId}-title`}>{fmt(m.comments.hideTitle, { noun, id: itemId })}</h2>
        <form method="dialog">
          <div className="field">
            <label htmlFor={`${dialogId}-reason`}>
              {m.comments.reason}{' '}
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
                {m.form.select}
              </option>
              {HIDE_REASONS.map((r) => (
                <option key={r} value={r}>
                  {m.comments.hideReasons[r]}
                </option>
              ))}
            </select>
            <span id={`${dialogId}-reason-error`} className="error-msg">
              {m.comments.reasonRequired}
            </span>
          </div>
          <div className="form-actions">
            {/* Chrome validates required controls even on method="dialog"
                submits — formnovalidate lets Cancel close without a reason. */}
            <button type="submit" className="btn" value="cancel" formNoValidate autoFocus>
              {m.copy.cancel}
            </button>
            <button type="submit" className="btn btn-primary" value="hide">
              {fmt(m.comments.hideSubmit, { noun })}
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
  const m = useMessages()
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
          {m.comments.bodyLabel}
        </label>
        <textarea
          id={`edit-${id}`}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={submitOnCtrlEnter}
          rows={4}
          required
          aria-label={m.comments.bodyLabel}
          data-error={`edit-${id}-error`}
        />
        <span id={`edit-${id}-error`} className="error-msg">
          {m.form.bodyRequired}
        </span>
      </div>
      <div className="form-actions">
        <button type="button" className="btn btn-quiet" onClick={onCancel}>
          {m.copy.cancel}
        </button>
        <button type="submit" className="btn btn-primary" disabled={pending}>
          {m.form.saveChanges}
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
  const m = useMessages()
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
        {fmt(m.comments.markedAs, { noun, reason: hiddenReason ?? 'hidden' })}
      </p>
      <details className="comment-hidden-body">
        <summary>{fmt(m.comments.show, { noun })}</summary>
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
  const m = useMessages()
  const fmtTime = useFmtTime()
  const noun = m.comments.nounReply
  return (
    <li
      className={`card comment comment-nested${c.hidden ? ' comment-hidden' : ''}`}
      id={`comment-${c.id}`}
    >
      <header className="comment-meta">
        <span className="comment-author">
          {fmt(m.comments.replied, { name: c.author?.name ?? m.comments.anonymous })}
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
      </header>
      {c.hidden ? (
        <ModeratedBody body={c.body} hidden noun={noun} hiddenReason={c.hidden_reason} />
      ) : ui.editing ? (
        <InlineEditForm
          label={fmt(m.comments.editFormAria, { noun, id: c.id })}
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
              aria-label={fmt(m.comments.ariaEdit, { noun, id: c.id })}
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
                  onClick={() => onUnhide(threadId, c.id)}
                  aria-label={fmt(m.comments.ariaUnhide, { noun, id: c.id })}
                >
                  {m.comments.unhide}
                </button>
              )
            : onHide && (
                <HideDialog
                  noun={noun}
                  itemId={c.id}
                  pending={pending}
                  onHide={(r) => onHide(threadId, c.id, r)}
                />
              )}
          {onDelete && (
            <ConfirmDialog
              label={m.comments.delete}
              title={fmt(m.comments.deleteTitle, { noun, id: c.id })}
              description={m.comments.deleteReplyBody}
              confirmLabel={fmt(m.comments.deleteConfirm, { noun })}
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
  const m = useMessages()
  const fmtTime = useFmtTime()
  const noun = m.comments.nounThread
  const permalink = () => `${location.origin}${location.pathname}#thread-${t.id}`
  return (
    <li
      className={`card comment${t.is_answer ? ' comment-answer' : ''}${t.hidden ? ' comment-hidden' : ''}`}
      id={`thread-${t.id}`}
    >
      <header className="comment-meta">
        {t.is_answer && <AnsweredBadge />}{' '}
        <span className="comment-author">
          {fmt(m.comments.commented, { name: t.author?.name ?? m.comments.anonymous })}
        </span>{' '}
        <a className="comment-permalink" href={`#thread-${t.id}`}>
          <time dateTime={t.created_at}>{fmtTime(t.created_at)}</time>
        </a>
        {t.edited_at && (
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
      {t.hidden ? (
        <ModeratedBody body={t.body} hidden noun={noun} hiddenReason={t.hidden_reason} />
      ) : ui.editing ? (
        <InlineEditForm
          label={fmt(m.comments.editFormAria, { noun, id: t.id })}
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
              aria-label={fmt(m.comments.ariaEdit, { noun, id: t.id })}
            >
              {m.comments.edit}
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
                {m.discussions.unmarkAnswer}
              </button>
            ) : (
              !t.hidden && (
                <button
                  type="button"
                  className="comment-action"
                  disabled={pending}
                  onClick={() => onSetAnswer(t.id, true)}
                >
                  {m.discussions.markAnswer}
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
              {m.comments.reply}
            </button>
          )}
          {t.hidden
            ? onUnhide && (
                <button
                  type="button"
                  className="comment-action"
                  disabled={pending}
                  onClick={() => onUnhide(t.id)}
                  aria-label={fmt(m.comments.ariaUnhide, { noun, id: t.id })}
                >
                  {m.comments.unhide}
                </button>
              )
            : onHide && (
                <HideDialog
                  noun={noun}
                  itemId={t.id}
                  pending={pending}
                  onHide={(r) => onHide(t.id, r)}
                />
              )}
          {onDelete && (
            <ConfirmDialog
              label={m.comments.delete}
              title={fmt(m.comments.deleteTitle, { noun, id: t.id })}
              description={m.comments.deleteThreadBody}
              confirmLabel={fmt(m.comments.deleteConfirm, { noun })}
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
          aria-label={fmt(m.comments.replyFormAria, { id: t.id })}
          onSubmit={(e) => {
            e.preventDefault()
            void onReply?.(t.id, ui.draft).then((ok) => {
              // submitted clears the draft; a failed send keeps it so the
              // reopened form restores what the user typed.
              dispatch({ type: ok ? 'submitted' : 'cancel' })
            })
          }}
        >
          <div className="field">
            <label htmlFor={`reply-${t.id}`} className="visually-hidden">
              {m.comments.replyLabel}
            </label>
            <textarea
              id={`reply-${t.id}`}
              value={ui.draft}
              onChange={(e) => dispatch({ type: 'draft', value: e.target.value })}
              onKeyDown={submitOnCtrlEnter}
              rows={3}
              required
              placeholder={m.comments.replyPh}
              data-error={`reply-${t.id}-error`}
            />
            <span id={`reply-${t.id}-error`} className="error-msg">
              {m.comments.replyRequired}
            </span>
          </div>
          <div className="form-actions">
            <button type="submit" className="btn" disabled={pending}>
              {m.comments.reply}
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
  const m = useMessages()
  if (threads.length === 0) {
    return <p className="muted">{m.comments.empty}</p>
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
  const m = useMessages()
  const fmtTime = useFmtTime()
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
          {fmt(m.issues.openedBy, { name: d.author?.name ?? m.comments.anonymous })}{' '}
          <time dateTime={d.created_at}>{fmtTime(d.created_at)}</time> · {m.issues.lastUpdated}{' '}
          <time dateTime={d.updated_at}>{fmtTime(d.updated_at)}</time>
        </p>
      </header>

      {d.body ? (
        <div className="issue-body">
          <Markdown text={d.body} />
        </div>
      ) : (
        <div className="issue-body issue-body-empty">
          <em>{m.issues.noDescription}</em>
        </div>
      )}

      <div className="issue-actions">
        <Link
          className="btn"
          to="/w/$ws/discussions/$discussionId/edit"
          params={{ ws, discussionId: String(d.id) }}
        >
          {m.comments.edit}
        </Link>
        {onCopyMarkdown && (
          <CopyButton label={m.copy.asMarkdown} getText={onCopyMarkdown} pending={pending} />
        )}
        {onDelete && (
          <ConfirmDialog
            label={m.discussions.delete}
            title={fmt(m.discussions.deleteTitle, { id: d.id })}
            description={fmt(m.discussions.deleteBody, { title: d.title })}
            confirmLabel={m.discussions.deleteConfirm}
            pending={pending}
            onConfirm={onDelete}
          />
        )}
      </div>
    </article>
  )
}
