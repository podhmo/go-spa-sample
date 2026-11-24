import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { render } from 'vitest-browser-react'
import { vi } from 'vitest'
import { routeTree } from './routes'

// renderAt mounts the real route tree on an in-memory history — component
// checks run inside the actual router so Link/loader/useSearch all work.
// The router rides along so tests can assert the landed location.
export function renderAt(initialPath: string) {
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  })
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

// stubAPI replaces fetch; handler maps (url, init) → response. Loaders hit
// it through the same fetchJSON path the app uses.
export function stubAPI(handler: (url: string, init?: RequestInit) => StubResponse) {
  return vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    const { status = 200, body, headers } = handler(url, init)
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
