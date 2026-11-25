import { useReducer } from 'react'
import { useMessages } from '../i18n'
import styles from './CopyButton.module.css'

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
  variant = 'action',
}: {
  label: string
  getText: () => string | Promise<string>
  pending?: boolean | undefined
  // `action` is the normal button primitive; `quiet` is the small
  // hover-revealed copy-link control inside comment headers.
  variant?: 'action' | 'quiet'
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
    <span className={styles.root} data-copy-wrap>
      <button
        type="button"
        className={variant === 'quiet' ? styles.quiet : styles.action}
        data-copy-variant={variant}
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
      <span className={styles.tip} role="status" data-shown={s.state !== 'idle' || undefined}>
        {s.state === 'idle' ? '' : s.state === 'copied' ? m.copy.copied : m.copy.failed}
      </span>
    </span>
  )
}
