import { useBlocker } from '@tanstack/react-router'

// Unsaved-changes guard (docs/ja/ui-rules.md §3): while `dirty`, blocks
// in-app navigation with a native confirm and browser close/reload via
// beforeunload. Call inside any form-bearing component; the guard
// releases once the form is submitted or the edits are reverted.
export function useLeaveGuard(dirty: boolean, message: string) {
  useBlocker({
    shouldBlockFn: () => !window.confirm(message),
    enableBeforeUnload: () => dirty,
    disabled: !dirty,
  })
}
