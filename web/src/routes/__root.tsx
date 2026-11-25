import {
  createRootRoute,
  Link,
  Outlet,
  useRouter,
  useLocation,
  useParams,
  useRouterState,
  isNotFound,
} from '@tanstack/react-router'
import type { ErrorComponentProps } from '@tanstack/react-router'
import { useReducer } from 'react'
import { api, type User } from '../api'
import { getLocale, setLocale, useMessages, type Locale } from '../i18n'
import { usePageTitle } from '../page-title'

// The site chrome: same as the MPA's layout.html — skip link, sticky
// header (site title + nav + primary action), main landmark, footer.
// Inside a workspace the nav points at that workspace's pages; outside
// one it uses the bare paths, which resolve to the home workspace.
function SiteNav({
  ws,
  pulls,
  user,
}: {
  ws: string | undefined
  pulls: boolean
  user: User | null
}) {
  const loc = useLocation()
  const m = useMessages()
  // Returning to the current page after login — /login & /register
  // themselves bounce back to '/' instead.
  const next = loc.pathname === '/login' || loc.pathname === '/register' ? undefined : loc.href
  const account = user ? (
    <>
      <span className="nav-user">{user.name}</span>
      <LogoutButton />
    </>
  ) : (
    <>
      <Link to="/login" search={{ next }}>
        {m.nav.logIn}
      </Link>
      <Link to="/register" search={{ next }}>
        {m.nav.register}
      </Link>
    </>
  )
  const langSwitch = (
    <>
      <label className="visually-hidden" htmlFor="lang-switch">
        {m.nav.language}
      </label>
      <select
        id="lang-switch"
        className="lang-switch"
        value={getLocale()}
        onChange={(e) => setLocale(e.target.value as Locale)}
      >
        <option value="en">English</option>
        <option value="ja">日本語</option>
      </select>
    </>
  )
  if (ws !== undefined) {
    return (
      <>
        <Link className="site-title" to="/w/$ws/issues" params={{ ws }}>
          zatudan
        </Link>
        <nav className="site-nav" aria-label={m.nav.aria}>
          <Link to="/w/$ws/issues" params={{ ws }}>
            {m.nav.issues}
          </Link>
          <Link to="/w/$ws/discussions" params={{ ws }}>
            {m.nav.discussions}
          </Link>
          {pulls && <Link to="/pulls">{m.nav.pulls}</Link>}
          <Link to="/w/$ws/labels" params={{ ws }}>
            {m.nav.labels}
          </Link>
          <Link to="/workspaces">{m.nav.workspaces}</Link>
          {langSwitch}
          <Link className="btn" to="/w/$ws/issues/new" params={{ ws }}>
            {m.nav.newIssue}
          </Link>
          {account}
        </nav>
      </>
    )
  }
  return (
    <>
      <Link className="site-title" to="/">
        zatudan
      </Link>
      <nav className="site-nav" aria-label={m.nav.aria}>
        <Link to="/issues">{m.nav.issues}</Link>
        <Link to="/discussions">{m.nav.discussions}</Link>
        {pulls && <Link to="/pulls">{m.nav.pulls}</Link>}
        <Link to="/labels">{m.nav.labels}</Link>
        <Link to="/workspaces">{m.nav.workspaces}</Link>
        {langSwitch}
        <Link className="btn" to="/issues/new">
          {m.nav.newIssue}
        </Link>
        {account}
      </nav>
    </>
  )
}

// POST /auth/logout, then re-resolve the root loader so nav and page
// state flip to anonymous.
function LogoutButton() {
  const router = useRouter()
  const [state, setState] = useReducer(
    (_: 'idle' | 'pending' | 'failed', v: 'idle' | 'pending' | 'failed') => v,
    'idle',
  )
  const m = useMessages()
  return (
    <>
      <button
        type="button"
        className="btn"
        disabled={state === 'pending'}
        onClick={() => {
          setState('pending')
          void (async () => {
            try {
              await api.logout()
              setState('idle')
              void router.invalidate()
            } catch {
              // The session may still be live server-side — say so
              // rather than silently re-arming the button.
              setState('failed')
            }
          })()
        }}
      >
        {m.nav.logOut}
      </button>
      {state === 'failed' && <em role="alert">{m.nav.logOutFailed}</em>}
    </>
  )
}

export const Route = createRootRoute({
  // Probes whether the server resolved a git repo; pulls routes 404
  // without one, so the nav link only renders when the API answers.
  // The session user rides along so the nav can greet them. staleTime
  // keeps plain navigations from re-probing; router.invalidate() still
  // re-runs both, so the link and the login state refresh after changes.
  staleTime: 60_000,
  loader: async () => {
    const [pulls, user] = await Promise.all([
      api.pullsAvailable(),
      // A broken session probe must not take the whole page down — the
      // nav simply falls back to anonymous.
      api.me().catch(() => null),
    ])
    return { pulls, user }
  },
  component: function RootLayout() {
    const { ws } = useParams({ strict: false })
    const { pulls, user } = Route.useLoaderData()
    const m = useMessages()
    const loc = useLocation()
    usePageTitle(loc.pathname, m)
    const isPending = useRouterState({ select: (s) => s.status === 'pending' })
    return (
      <>
        {isPending && <progress className="route-progress" aria-label={m.chrome.loading} />}
        <a className="skip-link" href="#main-content">
          {m.nav.skip}
        </a>
        <header className="site-header">
          <div className="header-bar">
            <div className="container header-inner">
              <SiteNav ws={ws} pulls={pulls} user={user} />
            </div>
          </div>
        </header>
        <main className="container main-content" id="main-content" tabIndex={-1}>
          <Outlet />
        </main>
        <footer className="site-footer">
          <div className="container">
            <p>{m.chrome.footerTagline}</p>
          </div>
        </footer>
      </>
    )
  },
  notFoundComponent: function NotFoundPage() {
    const m = useMessages()
    return (
      <div className="error-page">
        <p className="error-code">404</p>
        <h1>{m.error.notFoundTitle}</h1>
        <p>
          {m.error.notFoundBody} <Link to="/issues">{m.error.backToIssues}</Link>
        </p>
      </div>
    )
  },
  errorComponent: function ErrorPage({ error }: ErrorComponentProps) {
    const m = useMessages()
    return (
      <div className="error-page">
        <h1>{m.error.somethingWrong}</h1>
        {isNotFound(error) ? (
          <p>
            {m.error.notFoundBody} <Link to="/issues">{m.error.backToIssues}</Link>
          </p>
        ) : (
          <p role="alert">{error instanceof Error ? error.message : String(error)}</p>
        )}
      </div>
    )
  },
})
