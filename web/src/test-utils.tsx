import { RouterProvider, createMemoryHistory, createRouter } from '@tanstack/react-router'
import { render } from 'vitest-browser-react'
import { vi } from 'vitest'
import { routeTree } from './routes'

// renderAt mounts the real route tree on an in-memory history — component
// checks run inside the actual router so Link/loader/useSearch all work.
export function renderAt(initialPath: string) {
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  })
  return render(<RouterProvider router={router} />)
}

type StubResponse = { status?: number; body?: unknown }

// stubAPI replaces fetch; handler maps (url, init) → response. Loaders hit
// it through the same fetchJSON path the app uses.
export function stubAPI(handler: (url: string, init?: RequestInit) => StubResponse) {
  return vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    const { status = 200, body } = handler(url, init)
    return new Response(body === undefined ? null : JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json' },
    })
  })
}
