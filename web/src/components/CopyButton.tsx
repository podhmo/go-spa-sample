import { useReducer } from 'react'

type CopyState = 'idle' | 'copied' | 'failed'

// Clipboard copy button with transient feedback. getText may fetch the
// payload lazily (e.g. the markdown export endpoint) so it is only
// requested when the user actually copies.
// Feedback lives in a role="status" bubble *outside* the button — a live
// region inside a button is not reliably announced.
export function CopyButton({
  label,
  getText,
  pending,
}: {
  label: string
  getText: () => string | Promise<string>
  pending?: boolean
}) {
  const [s, dispatch] = useReducer(
    (
      s: { state: CopyState; timer?: ReturnType<typeof setTimeout> },
      a: { state: CopyState; timer?: ReturnType<typeof setTimeout> },
    ) => {
      if (s.timer) clearTimeout(s.timer)
      return { state: a.state, timer: a.timer }
    },
    { state: 'idle' } as { state: CopyState; timer?: ReturnType<typeof setTimeout> },
  )
  return (
    <span className="copy">
      <button
        type="button"
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
      <span className="copy-tip" role="status" data-shown={s.state !== 'idle' || undefined}>
        {s.state === 'idle' ? '' : s.state}
      </span>
    </span>
  )
}
