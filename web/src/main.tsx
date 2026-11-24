import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import './index.css'
import { setupFieldErrorSync } from './a11y'
import { resetLeaveGuardLatch } from './leave-guard'
import { routeTree } from './routes.tsx'

setupFieldErrorSync()

const router = createRouter({
  routeTree,
  // MPA freebies the SPA has to opt into: browser-grade scroll
  // restoration, and hover/focus-intent preloading of the target
  // route's loader so navigations land warm.
  scrollRestoration: true,
  defaultPreload: 'intent',
})

// Full-page loads move focus to the top of the new document; a
// client-side nav does not, so move it to the main landmark when the
// path (not just search or hash) changes.
router.subscribe('onResolved', ({ fromLocation, toLocation }) => {
  // A completed navigation re-arms the leave-guard latch (leave-guard.ts).
  resetLeaveGuardLatch()
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
