import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  useRouter,
} from '@tanstack/react-router'

type Ping = { message: string; time: string }
type Counter = { count: number }

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) throw new Error(`${init?.method ?? 'GET'} ${path}: ${res.status}`)
  return res.json() as Promise<T>
}

const rootRoute = createRootRoute({
  component: () => (
    <>
      <nav style={{ display: 'flex', gap: '1rem' }}>
        <Link to="/">Home</Link>
        <Link to="/counter">Counter</Link>
      </nav>
      <hr />
      <Outlet />
    </>
  ),
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  loader: () => fetchJSON<Ping>('/api/ping'),
  component: function Index() {
    const ping = indexRoute.useLoaderData()
    return (
      <div>
        <h2>Ping</h2>
        <p>
          {ping.message} @ {ping.time}
        </p>
      </div>
    )
  },
})

const counterRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/counter',
  loader: () => fetchJSON<Counter>('/api/counter'),
  component: function Counter() {
    const { count } = counterRoute.useLoaderData()
    const router = useRouter()
    return (
      <div>
        <h2>Counter</h2>
        <p>count: {count}</p>
        <button
          onClick={async () => {
            await fetchJSON('/api/counter/increment', { method: 'POST' })
            await router.invalidate()
          }}
        >
          increment
        </button>
      </div>
    )
  },
})

export const routeTree = rootRoute.addChildren([indexRoute, counterRoute])
