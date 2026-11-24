import type { KeyboardEvent } from 'react'

// Ctrl+Enter / Cmd+Enter submits the enclosing form from a textarea —
// the React equivalent of the MPA's delegated enter-submit.js listener.
// Modifier combos do not conflict with IME composition, so no
// isComposing checks are needed.
export const submitOnCtrlEnter = (e: KeyboardEvent<HTMLTextAreaElement>) => {
  if (e.key !== 'Enter' || !(e.ctrlKey || e.metaKey)) return
  e.preventDefault()
  e.currentTarget.form?.requestSubmit()
}
