import { useReducer } from 'react'

type CopyState = 'idle' | 'copied' | 'failed'

// Clipboard copy button with transient feedback. getText may fetch the
// payload lazily (e.g. the markdown export endpoint) so it is only
// requested when the user actually copies.
export function CopyButton({
  label,
  getText,
  pending,
}: {
  label: string
  getText: () => string | Promise<string>
  pending?: boolean
}) {
  const [state, dispatch] = useReducer((_: CopyState, next: CopyState) => next, 'idle')
  return (
    <button
      type="button"
      disabled={pending}
      onClick={() => {
        void (async () => {
          try {
            await navigator.clipboard.writeText(await getText())
            dispatch('copied')
          } catch {
            dispatch('failed')
          }
          setTimeout(() => dispatch('idle'), 1500)
        })()
      }}
    >
      {state === 'idle' ? label : state}
    </button>
  )
}
