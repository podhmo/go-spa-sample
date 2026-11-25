import { useId } from 'react'
import { HIDE_REASONS } from '../api'
import { fmt, useMessages } from '../i18n'
import dialogStyles from './ConfirmDialog.module.css'

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
        className={dialogStyles.dialog}
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
