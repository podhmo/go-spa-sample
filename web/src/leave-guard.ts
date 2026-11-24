import { useBlocker } from '@tanstack/react-router'

// One latch shared by every guard on the page: the first accepted
// confirm releases the whole navigation (two dirty drafts must not ask
// twice), and the latch re-arms once the router settles on the next page
// (resetLeaveGuardLatch, called from main.tsx's onResolved subscriber).
const latch = { approved: false }
export function resetLeaveGuardLatch() {
  latch.approved = false
}

// A completed navigation re-arms the latch. main.tsx installs this on the
// real router and test-utils.renderAt installs it on the test router, so
// every environment gets the same reset-on-navigation behavior.
export function watchLeaveGuardReset(router: {
  subscribe: (event: 'onResolved', fn: () => void) => unknown
}) {
  router.subscribe('onResolved', resetLeaveGuardLatch)
}

// Unsaved-changes guard (docs/ja/ui-rules.md §3): while `dirty`, blocks
// in-app navigation with a native confirm and browser close/reload via
// beforeunload. Call inside any form-bearing component; the guard
// releases once the form is submitted or the edits are reverted.
export function useLeaveGuard(dirty: boolean, message: string) {
  useBlocker({
    shouldBlockFn: ({ current, next }) => {
      // Hash-only moves (permalink jumps after posting) leave the page's
      // data behind untouched — they are not "leaving".
      if (
        next.pathname === current.pathname &&
        JSON.stringify(next.search) === JSON.stringify(current.search)
      ) {
        return false
      }
      if (latch.approved) return false
      const allow = window.confirm(message)
      if (allow) latch.approved = true
      return !allow
    },
    enableBeforeUnload: () => dirty,
    disabled: !dirty,
  })
}
