import { useId } from 'react'

// Native <dialog> confirmation. The trigger opens it declaratively via
// Invoker Commands (command/commandfor); on close, returnValue tells which
// button was used ('' = dismissed without choosing).
export function ConfirmDialog({
  label,
  title,
  confirmLabel = 'confirm',
  pending,
  onConfirm,
}: {
  label: string
  title: string
  confirmLabel?: string
  pending?: boolean | undefined
  onConfirm: () => void
}) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const dialogId = `confirm-${id}`
  return (
    <>
      <button
        type="button"
        {...({ commandfor: dialogId, command: 'show-modal' } as Record<string, string>)}
        disabled={pending}
      >
        {label}
      </button>
      <dialog
        id={dialogId}
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
        <h1 id={`${dialogId}-title`}>{title}</h1>
        <form method="dialog">
          {/* safe action first: it also receives initial focus */}
          <button type="submit" value="cancel">
            cancel
          </button>{' '}
          <button type="submit" value="confirm">
            {confirmLabel}
          </button>
        </form>
      </dialog>
    </>
  )
}
