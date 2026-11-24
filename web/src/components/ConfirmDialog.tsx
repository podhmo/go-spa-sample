import { useId } from 'react'

// Native <dialog> confirmation, styled like the MPA's .confirm-dialog.
// The trigger opens it declaratively via Invoker Commands
// (command/commandfor); on close, returnValue tells which button was
// used ('' = dismissed without choosing). The safe action (cancel) is
// first and takes initial focus, per the MPA's delete-dialog pattern.
export function ConfirmDialog({
  label,
  title,
  description,
  confirmLabel = 'Confirm',
  triggerClass = 'btn btn-danger',
  pending,
  onConfirm,
}: {
  label: string
  title: string
  // Optional explanation under the title (the MPA's "This permanently
  // removes …" paragraph).
  description?: string | undefined
  confirmLabel?: string
  triggerClass?: string
  pending?: boolean | undefined
  onConfirm: () => void
}) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const dialogId = `confirm-${id}`
  return (
    <>
      <button
        type="button"
        className={triggerClass}
        {...({ commandfor: dialogId, command: 'show-modal' } as Record<string, string>)}
        disabled={pending}
      >
        {label}
      </button>
      <dialog
        id={dialogId}
        className="confirm-dialog"
        aria-labelledby={`${dialogId}-title`}
        ref={(el) => {
          // closedby isn't in React's types yet; Esc still closes modals without it.
          if (el && !el.hasAttribute('closedby')) el.setAttribute('closedby', 'any')
        }}
        onClose={(e) => {
          const d = e.currentTarget
          if (d.returnValue === 'confirm') onConfirm()
          d.returnValue = ''
        }}
      >
        <h2 id={`${dialogId}-title`}>{title}</h2>
        {description && <p>{description}</p>}
        <form method="dialog" className="form-actions">
          {/* safe action first: it also receives initial focus */}
          <button type="submit" className="btn" value="cancel" autoFocus>
            Cancel
          </button>
          <button type="submit" className="btn btn-danger" value="confirm">
            {confirmLabel}
          </button>
        </form>
      </dialog>
    </>
  )
}
