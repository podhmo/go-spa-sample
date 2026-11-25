import { useReducer } from 'react'
import { submitOnCtrlEnter } from '../enter-submit'
import { useMessages } from '../i18n'
import { useLeaveGuard } from '../leave-guard'
import styles from './CommentForm.module.css'
import { cx } from '../styles/cx'

export function CommentForm({
  onSubmit,
  submitting,
  error,
}: {
  onSubmit: (body: string) => void | boolean | Promise<void | boolean>
  submitting?: boolean
  error?: string | null | undefined
}) {
  const [body, setBody] = useReducer((_: string, v: string) => v, '')
  const [sending, setSending] = useReducer((_: boolean, v: boolean) => v, false)
  const busy = submitting === true || sending
  const m = useMessages()
  useLeaveGuard(body.trim() !== '', m.form.unsavedChanges)
  return (
    <form
      className={styles.form}
      aria-label={m.form.newCommentAria}
      onSubmit={(e) => {
        e.preventDefault()
        const form = e.currentTarget
        void (async () => {
          setSending(true)
          try {
            const ok = await onSubmit(body)
            if (ok !== false) {
              // Native reset clears the :user-invalid interaction state;
              // clearing only the controlled value leaves it looking invalid.
              form.reset()
              setBody('')
            }
          } finally {
            setSending(false)
          }
        })()
      }}
    >
      <div className={styles.formField}>
        <label htmlFor="comment-body">
          {m.form.addComment}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <textarea
          id="comment-body"
          className={styles.body}
          name="body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={submitOnCtrlEnter}
          rows={4}
          required
          disabled={busy}
          data-error="comment-body-error"
          aria-invalid={error ? 'true' : 'false'}
          aria-describedby={
            error ? 'comment-body-api-error comment-body-hint' : 'comment-body-hint'
          }
        />
        <span id="comment-body-error" className={cx('error-msg')}>
          {m.form.bodyRequired}
        </span>
        {error && (
          <em className={cx('error-msg')} id="comment-body-api-error" role="alert">
            {error}
          </em>
        )}
        <p className={cx('field-hint')} id="comment-body-hint">
          {m.form.bodyHintShort}
        </p>
      </div>
      <div className={cx('form-actions')}>
        {/* disabled while empty: an empty post can only 422, so the
            GitHub-style rule disables the button until there is input
            (ui-rules §6) */}
        <button type="submit" className={cx('btn btn-primary')} disabled={busy || !body.trim()}>
          {busy ? m.form.posting : m.pulls.commentSubmit}
        </button>
      </div>
    </form>
  )
}
