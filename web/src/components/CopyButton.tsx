import { useReducer } from 'react'
import { useMessages } from '../i18n'
import { cx } from '../styles/cx'

type CopyState = 'idle' | 'copied' | 'failed'
// `timer` is present-but-undefined after a dispatch without one
// (exactOptionalPropertyTypes).
type CopyUIState = { state: CopyState; timer?: ReturnType<typeof setTimeout> | undefined }

// Clipboard copy button with transient feedback. getText may fetch the
// payload lazily (e.g. the markdown export endpoint) so it is only
// requested when the user actually copies.
// Feedback lives in a role="status" bubble *outside* the button — a
// live region inside a button is not reliably announced. The state is
// also mirrored on the button as data-copy-state for the MPA's
// copied/failed border colors.
export function CopyButton({
  label,
  getText,
  pending,
  className = cx('btn'),
}: {
  label: string
  getText: () => string | Promise<string>
  pending?: boolean | undefined
  // '.btn' for action rows (Copy as markdown), '.copy-btn' for the
  // quiet hover-revealed button on comment headers (copy link).
  className?: string
}) {
  const [s, dispatch] = useReducer(
    (s: CopyUIState, a: CopyUIState) => {
      if (s.timer) clearTimeout(s.timer)
      return { state: a.state, timer: a.timer }
    },
    { state: 'idle' } as CopyUIState,
  )
  const m = useMessages()
  return (
    <span className={cx('copy-wrap')}>
      <button
        type="button"
        className={className}
        data-copy-state={s.state === 'idle' ? undefined : s.state}
        disabled={pending}
        onClick={() => {
          void (async () => {
            let next: CopyState = 'copied'
            try {
              await navigator.clipboard.writeText(await getText())
            } catch {
              next = 'failed'
            }
            dispatch({
              state: next,
              timer: setTimeout(() => dispatch({ state: 'idle' }), 1500),
            })
          })()
        }}
      >
        {label}
      </button>{' '}
      <span className={cx('copy-tip')} role="status" data-shown={s.state !== 'idle' || undefined}>
        {s.state === 'idle' ? '' : s.state === 'copied' ? m.copy.copied : m.copy.failed}
      </span>
    </span>
  )
}
