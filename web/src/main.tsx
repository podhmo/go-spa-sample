import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import './index.css'
import { setupFieldErrorSync } from './a11y'
import { watchLeaveGuardReset } from './leave-guard'
import { watchLiveUpdates } from './live-updates'
import { routeTree } from './routeTree.gen'

setupFieldErrorSync()

const router = createRouter({
  routeTree,
  // MPA freebies the SPA has to opt into: browser-grade scroll
  // restoration, and hover/focus-intent preloading of the target
  // route's loader so navigations land warm.
  scrollRestoration: true,
  defaultPreload: 'intent',
})

// Live updates: writes from anywhere — including the local CLI in
// another process — land in the changes table, stream out of
// /api/v1/events, and invalidate the active loaders (live-updates.ts).
watchLiveUpdates(router)

// Full-page loads move focus to the top of the new document; a
// client-side nav does not, so move it to the main landmark when the
// path (not just search or hash) changes.
// A completed navigation re-arms the leave-guard latch (leave-guard.ts).
watchLeaveGuardReset(router)
router.subscribe('onResolved', ({ fromLocation, toLocation }) => {
  if (fromLocation && fromLocation.pathname !== toLocation.pathname) {
    requestAnimationFrame(() => {
      document.getElementById('main-content')?.focus({ preventScroll: true })
    })
  }
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
