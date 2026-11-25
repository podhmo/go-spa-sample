import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { render } from 'vitest-browser-react'
import { vi } from 'vitest'
import { setupFieldErrorSync } from './a11y'
import { watchLeaveGuardReset } from './leave-guard'
import { routeTree } from './routeTree.gen'

// renderAt mounts the real route tree on an in-memory history — component
// checks run inside the actual router so Link/loader/useSearch all work.
// The router rides along so tests can assert the landed location.
export function renderAt(initialPath: string) {
  setupFieldErrorSync()
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  })
  watchLeaveGuardReset(router)
  return { router, ...render(<RouterProvider router={router} />) }
}

// renderWithRouter wraps a lone component in a minimal router so
// <Link>-bearing components (issue rows, forms) render outside the app.
export function renderWithRouter(ui: ReactNode) {
  const rootRoute = createRootRoute({ component: () => ui })
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(<RouterProvider router={router} />)
}

type StubResponse = { status?: number; body?: unknown; headers?: Record<string, string> }

// Async responses model an in-flight request (e.g. testing double-
// submit guards against a pending POST).
type StubHandler = (url: string, init?: RequestInit) => StubResponse | Promise<StubResponse>

// stubAPI replaces fetch; handler maps (url, init) → response. Loaders hit
// it through the same fetchJSON path the app uses. /auth/me is answered
// anonymously up front so handlers that expect only their own URLs don't
// have to know about the session probe; auth tests pass `me` to model
// the cookie.
export function stubAPI(handler: StubHandler, opts?: { me?: () => StubResponse }) {
  return vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    const {
      status = 200,
      body,
      headers,
    } = url.endsWith('/api/v1/auth/me')
      ? (opts?.me?.() ?? { status: 401, body: {} })
      : await handler(url, init)
    // String bodies go out raw (text endpoints like /diff or /markdown);
    // everything else is JSON.
    if (typeof body === 'string') {
      return new Response(body, {
        status,
        headers: { 'content-type': 'text/plain', ...headers },
      })
    }
    return new Response(body === undefined ? null : JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json', ...headers },
    })
  })
}
