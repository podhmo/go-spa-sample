import { useEffect, useReducer, useRef } from 'react'
import { useLeaveGuard } from '../../../../leave-guard'
import { RequestError, type PRCommentInput } from '../../../../api'
import { submitOnCtrlEnter } from '../../../../enter-submit'
import { fmt, useMessages } from '../../../../i18n'
import { parseDiff, type DiffLine } from '../-lib/diff'
import styles from './PullFiles.module.css'

// The open draft anchors to a coordinate AND that row's code when the
// form opened (without the +/-/space diff marker — a rebase can turn an
// added line into context without changing its source text). A
// rewritten diff can put different code at the same path:side:line, and
// a retry must not attach the draft to it.
type FormUI = { openAt: { key: string; text: string } | null }
type FormAction = { type: 'toggle'; open: { key: string; text: string } | null }

// The files tab: unified diff with a comment button on every real line,
// expanding an inline form that posts {body, path, line, side}.
export function PullFiles({
  diff,
  pending,
  onComment,
}: {
  diff: string
  pending?: boolean | undefined
  // Rejected posts propagate: DiffRow keeps the form open with the
  // draft and shows the API's error (e.g. a stale anchor after a
  // force-push).
  onComment?: (input: PRCommentInput) => Promise<void> | void
}) {
  const [ui, dispatch] = useReducer(
    (s: FormUI, a: FormAction): FormUI => ({
      openAt: s.openAt?.key === a.open?.key ? null : a.open,
    }),
    { openAt: null },
  )
  const files = parseDiff(diff)
  // A different diff (force-push, new commits) can shift every row. An
  // open draft whose anchor row disappeared — or whose row now holds
  // different code — no longer points at the code it was written for:
  // close it rather than let a retry attach it to replacement code.
  useEffect(() => {
    if (ui.openAt === null) return
    const row = files
      .flatMap((f) => f.lines.map((l, i) => ({ key: `${f.path}:${l.side}:${l.line ?? i}`, l })))
      .find((r) => r.key === ui.openAt?.key)
    if (!row || row.l.text.slice(1) !== ui.openAt.text) {
      dispatch({ type: 'toggle', open: null })
    }
    // `files` reparses every render; diff + openAt are the triggers.
    // oxlint-disable-next-line react/exhaustive-deps
  }, [diff, ui.openAt])
  const m = useMessages()
  if (files.length === 0) {
    return <p className="muted">{m.pulls.noChanges}</p>
  }
  return (
    <div className={styles.diffFiles}>
      {files.map((f) => (
        <section key={f.path}>
          <h2 className={styles.diffFileName}>
            <code>{f.path}</code>
          </h2>
          <table className={styles.diffTable}>
            <tbody>
              {f.lines.map((l, i) => {
                const key = `${f.path}:${l.side}:${l.line ?? i}`
                const formOpen = ui.openAt?.key === key
                return (
                  <DiffRow
                    key={key}
                    line={l}
                    formOpen={formOpen}
                    pending={pending}
                    canComment={onComment !== undefined && l.path !== undefined}
                    onToggle={() =>
                      dispatch({ type: 'toggle', open: { key, text: l.text.slice(1) } })
                    }
                    onSubmit={async (body) => {
                      if (l.path && l.line && l.side) {
                        await onComment?.({ body, path: l.path, line: l.line, side: l.side })
                      }
                    }}
                  />
                )
              })}
            </tbody>
          </table>
        </section>
      ))}
    </div>
  )
}

function DiffRow({
  line,
  formOpen,
  pending,
  canComment,
  onToggle,
  onSubmit,
}: {
  line: DiffLine
  formOpen: boolean
  pending?: boolean | undefined
  canComment: boolean
  onToggle: () => void
  // Resolves when the comment posted; rejection keeps the draft and
  // shows the error.
  onSubmit: (body: string) => Promise<void> | void
}) {
  const [postError, setPostError] = useReducer((_: string | null, v: string | null) => v, null)
  const [dirty, setDirty] = useReducer((_: boolean, v: boolean) => v, false)
  const [blank, setBlank] = useReducer((_: boolean, v: boolean) => v, true)
  const m = useMessages()
  useLeaveGuard(formOpen && dirty, m.form.unsavedChanges)
  // The disabled button only blocks clicks: Ctrl+Enter (requestSubmit)
  // still fires, so the send itself must be re-entrant-safe.
  const sending = useRef(false)
  // The textarea is uncontrolled, so closing the form discards the DOM
  // draft: every toggle must reset the flags or a reopen shows an empty
  // box with Comment enabled / a phantom dirty guard.
  const toggle = () => {
    setDirty(false)
    setBlank(true)
    setPostError(null)
    onToggle()
  }
  if (line.kind === 'meta') {
    return (
      <tr className={`${styles.diffLine} ${styles.diffMeta}`}>
        <td className={styles.diffCommentCell} />
        <td colSpan={2}>
          <code>{line.text}</code>
        </td>
      </tr>
    )
  }
  const kindClasses = {
    ctx: '',
    add: styles.diffAdd,
    del: styles.diffDel,
    meta: styles.diffMeta,
  } as const
  return (
    <>
      <tr className={`${styles.diffLine} ${kindClasses[line.kind]}`}>
        <td className={styles.diffCommentCell}>
          {canComment && (
            <button
              type="button"
              className={styles.diffAddComment}
              onClick={toggle}
              aria-expanded={formOpen}
              aria-label={fmt(m.pulls.commentOnLine, {
                path: line.path ?? '',
                line: line.line ?? 0,
                side: line.side ?? '',
              })}
            >
              +
            </button>
          )}
        </td>
        <td className={styles.diffLineno}>
          <code>{line.line}</code>
        </td>
        <td className="diff-text">
          <code>{line.text}</code>
        </td>
      </tr>
      {formOpen && (
        <tr className={`${styles.diffLine} ${styles.diffCommentForm}`}>
          <td />
          <td colSpan={2}>
            <form
              aria-label={fmt(m.pulls.commentOnLineAria, {
                path: line.path ?? '',
                line: line.line ?? 0,
              })}
              onSubmit={(e) => {
                e.preventDefault()
                if (sending.current) return
                sending.current = true
                const body = String(new FormData(e.currentTarget).get('body') ?? '')
                setPostError(null)
                // Clear the guard before awaiting: the route navigates
                // to the new comment inside onSubmit, and a still-dirty
                // flag would intercept that redirect.
                setDirty(false)
                void (async () => {
                  try {
                    await onSubmit(body)
                    toggle()
                  } catch (err) {
                    // The draft is still in the textarea — re-arm.
                    setDirty(true)
                    // Field errors name the actual rejection reason
                    // (e.g. a stale anchor); a bare message just says
                    // "Validation failed.".
                    const fields = err instanceof RequestError ? err.apiError?.fields : undefined
                    setPostError(
                      fields
                        ? Object.values(fields).join(' ')
                        : err instanceof Error
                          ? err.message
                          : m.pulls.commentFailed,
                    )
                  } finally {
                    sending.current = false
                  }
                })()
              }}
            >
              <div className="field">
                <label
                  htmlFor={`dc-${line.path}-${line.side}-${line.line}`}
                  className="visually-hidden"
                >
                  {m.pulls.comment}
                </label>
                <textarea
                  id={`dc-${line.path}-${line.side}-${line.line}`}
                  name="body"
                  rows={3}
                  required
                  onChange={(e) => {
                    setDirty(e.target.value.trim() !== '')
                    setBlank(e.target.value.trim() === '')
                  }}
                  onKeyDown={submitOnCtrlEnter}
                  data-error={`dc-${line.path}-${line.line}-error`}
                  placeholder={m.pulls.reviewCommentPh}
                />
                <span id={`dc-${line.path}-${line.line}-error`} className="error-msg">
                  {m.form.bodyRequired}
                </span>
                {postError && <em role="alert">{postError}</em>}
              </div>
              <div className="form-actions">
                <button type="button" className="btn btn-quiet" onClick={toggle}>
                  {m.copy.cancel}
                </button>
                <button type="submit" className="btn btn-primary" disabled={pending || blank}>
                  {m.pulls.commentSubmit}
                </button>
              </div>
            </form>
          </td>
        </tr>
      )}
    </>
  )
}
