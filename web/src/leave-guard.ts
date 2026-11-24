import { useBlocker } from '@tanstack/react-router'

// One latch shared by every guard on the page: the first accepted
// confirm releases the whole navigation (two dirty drafts must not ask
// twice), and the latch re-arms once the router settles on the next page
// (resetLeaveGuardLatch, called from main.tsx's onResolved subscriber).
const latch = { approved: false }
export function resetLeaveGuardLatch() {
  latch.approved = false
}

// Unsaved-changes guard (docs/ja/ui-rules.md §3): while `dirty`, blocks
// in-app navigation with a native confirm and browser close/reload via
// beforeunload. Call inside any form-bearing component; the guard
// releases once the form is submitted or the edits are reverted.
export function useLeaveGuard(dirty: boolean, message: string) {
  useBlocker({
    shouldBlockFn: () => {
      if (latch.approved) return false
      const allow = window.confirm(message)
      if (allow) latch.approved = true
      return !allow
    },
    enableBeforeUnload: () => dirty,
    disabled: !dirty,
  })
}
