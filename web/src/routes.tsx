import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  isNotFound,
  notFound,
  redirect,
  useLocation,
  useNavigate,
  useParams,
  useRouter,
  useRouterState,
  type ErrorComponentProps,
} from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { RequestError, api, type PRCommentInput, type User } from './api'
import {
  AuthForm,
  CommentForm,
  DiscussionForm,
  IssueForm,
  PullForm,
  WorkspaceForm,
} from './components/forms'
import { CommentList, IssueDetail } from './components/IssueDetail'
import { IssueList } from './components/IssueList'
import { DiscussionList } from './components/DiscussionList'
import { DiscussionDetail, ThreadList } from './components/DiscussionDetail'
import { LabelsAdmin } from './components/LabelsAdmin'
import { PullList } from './components/PullList'
import { PRCommentList, PullCommitList, PullDetail } from './components/PullDetail'
import { PullFiles } from './components/PullFiles'
import { WorkspaceList } from './components/Workspaces'
import { fmt, getLocale, setLocale, useMessages, type Locale } from './i18n'
import { usePageTitle } from './page-title'

// A scoped API call: a plain 404 becomes the router's notFound, while a
// cross-scope 404 names the owning workspace in error.workspace — the
// API's in-band counterpart of the MPA's 302 — and rebase() throws a
// redirect to the same page under that slug.
const scoped = async <T,>(p: Promise<T>, rebase: (real: string) => never): Promise<T> => {
  try {
    return await p
  } catch (err) {
    if (err instanceof RequestError && err.status === 404) {
      const real = err.apiError?.workspace
      if (typeof real === 'string') rebase(real)
      throw notFound()
    }
    throw err
  }
}

// Where a workspace-less URL lands: 'default' when it exists, else the
// first workspace (the MPA's rule); null when none exist at all.
const homeSlug = async (): Promise<string | null> => {
  const { workspaces } = await api.listWorkspaces()
  return (workspaces.find((w) => w.slug === 'default') ?? workspaces[0])?.slug ?? null
}

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

const rootRoute = createRootRoute({
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
    const { pulls, user } = rootRoute.useLoaderData()
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

// --- Workspace-less compatibility routes ---------------------------------
// Pre-workspace URLs still work: each redirects to the same path under
// the resolved home workspace, like the MPA's 302 on /issues etc.

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: async () => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/issues', params: { ws } } : { to: '/workspaces' })
  },
})

// Keys are always present in the validated/search-prop objects but may
// hold undefined (exactOptionalPropertyTypes), so each is `T | undefined`.
type IssueSearch = {
  status?: string | undefined
  q?: string | undefined
  label?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

// TanStack Router decodes `?q=404`-style values to numbers and
// `?q=true` to booleans — coerce those back so numeric keywords survive.
const str = (v: unknown) =>
  typeof v === 'string' && v !== ''
    ? v
    : typeof v === 'number' && Number.isFinite(v)
      ? String(v)
      : typeof v === 'boolean'
        ? String(v)
        : undefined
const num = (v: unknown) => {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number(v) : NaN
  return Number.isInteger(n) && n >= 1 ? n : undefined
}

const issueSearch = (search: Record<string, unknown>): IssueSearch => ({
  status: str(search.status),
  q: str(search.q),
  label: str(search.label),
  sort: str(search.sort),
  page: num(search.page),
})

const issuesRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues',
  validateSearch: issueSearch,
  beforeLoad: async ({ search }) => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/issues', params: { ws }, search } : { to: '/workspaces' })
  },
})

const newIssueRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues/new',
  beforeLoad: async () => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/issues/new', params: { ws } } : { to: '/workspaces' })
  },
})

const issueRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues/$issueId',
  beforeLoad: async ({ params, location }) => {
    const ws = await homeSlug()
    // location.hash keeps a #comment-N permalink across the redirect.
    throw redirect(
      ws
        ? {
            to: '/w/$ws/issues/$issueId',
            params: { ws, issueId: params.issueId },
            hash: location.hash,
          }
        : { to: '/workspaces' },
    )
  },
})

const editIssueRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues/$issueId/edit',
  beforeLoad: async ({ params }) => {
    const ws = await homeSlug()
    throw redirect(
      ws
        ? { to: '/w/$ws/issues/$issueId/edit', params: { ws, issueId: params.issueId } }
        : { to: '/workspaces' },
    )
  },
})

type DiscussionSearch = {
  category?: string | undefined
  answered?: string | undefined
  q?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

const discussionSearch = (search: Record<string, unknown>): DiscussionSearch => ({
  category: str(search.category),
  answered: str(search.answered),
  q: str(search.q),
  sort: str(search.sort),
  page: num(search.page),
})

const discussionsRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/discussions',
  validateSearch: discussionSearch,
  beforeLoad: async ({ search }) => {
    const ws = await homeSlug()
    throw redirect(
      ws ? { to: '/w/$ws/discussions', params: { ws }, search } : { to: '/workspaces' },
    )
  },
})

const newDiscussionRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/discussions/new',
  beforeLoad: async () => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/discussions/new', params: { ws } } : { to: '/workspaces' })
  },
})

const discussionRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/discussions/$discussionId',
  beforeLoad: async ({ params, location }) => {
    const ws = await homeSlug()
    // #thread-N / #comment-N permalinks survive the redirect.
    throw redirect(
      ws
        ? {
            to: '/w/$ws/discussions/$discussionId',
            params: { ws, discussionId: params.discussionId },
            hash: location.hash,
          }
        : { to: '/workspaces' },
    )
  },
})

const editDiscussionRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/discussions/$discussionId/edit',
  beforeLoad: async ({ params }) => {
    const ws = await homeSlug()
    throw redirect(
      ws
        ? {
            to: '/w/$ws/discussions/$discussionId/edit',
            params: { ws, discussionId: params.discussionId },
          }
        : { to: '/workspaces' },
    )
  },
})

const labelsRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/labels',
  beforeLoad: async () => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/labels', params: { ws } } : { to: '/workspaces' })
  },
})

// --- Workspace management pages ------------------------------------------

const workspacesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/workspaces',
  loader: () => api.listWorkspaces(),
  component: function WorkspacesPage() {
    const { workspaces } = workspacesRoute.useLoaderData()
    const router = useRouter()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [error, setError] = useReducer((_: string | null, v: string | null) => v, null)
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      void (async () => {
        try {
          await fn()
          setError(null)
          void router.invalidate()
        } catch (err) {
          setError(err instanceof Error ? err.message : String(err))
        } finally {
          setPending(false)
        }
      })()
    }
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.workspaces.title}</h1>
          <Link className="btn" to="/workspaces/new">
            {m.workspaces.new}
          </Link>
        </div>
        {/* em role=alert is the always-visible server-error idiom;
            .error-msg alone stays hidden (field-sibling reveal only) */}
        {error && <em role="alert">{error}</em>}
        <WorkspaceList
          workspaces={workspaces}
          pending={isPending}
          onDelete={(slug) => mutate(() => api.deleteWorkspace(slug))}
        />
      </div>
    )
  },
})

// Same-site redirect target after login/registration: absolute paths
// only — '//evil' is a protocol-relative URL and some browsers treat
// '\' as '/', both escapes a naive startsWith('/') check would miss.
const safeNext = (raw: string | undefined): string =>
  raw !== undefined && raw.startsWith('/') && !raw.startsWith('//') && !raw.includes('\\')
    ? raw
    : '/'

const authSearch = (search: Record<string, unknown>) => ({ next: str(search.next) })

// /login and /register share this page; only the endpoint, autocomplete
// hints, and cross-link differ. A signed-in visitor is bounced to `next`
// by beforeLoad.
function AuthPage({ mode, next }: { mode: 'login' | 'register'; next: string | undefined }) {
  const router = useRouter()
  const [pending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
  const [errors, setErrors] = useReducer(
    (
      _: { name?: string; password?: string; form?: string },
      v: { name?: string; password?: string; form?: string },
    ) => v,
    {},
  )
  const m = useMessages()
  return (
    <div>
      <div className="page-head">
        <h1>{mode === 'login' ? m.nav.logIn : m.nav.register}</h1>
      </div>
      <AuthForm
        mode={mode}
        errors={errors}
        submitting={pending}
        onSubmit={(name, password) => {
          setPending(true)
          setErrors({})
          void (async () => {
            try {
              if (mode === 'login') await api.login(name, password)
              else await api.register(name, password)
              // The session cookie just landed. invalidate() re-runs
              // this route's beforeLoad, which now sees the session and
              // bounces to `next` — no explicit navigation (a second
              // push would leave a duplicate history entry).
              await router.invalidate()
            } catch (err) {
              // 422 field errors light up their inputs; the credential
              // failure deliberately carries none, so its message fills
              // the form-level slot.
              if (err instanceof RequestError && err.apiError?.fields) {
                setErrors(err.apiError.fields)
              } else {
                setErrors({ form: err instanceof Error ? err.message : String(err) })
              }
            } finally {
              setPending(false)
            }
          })()
        }}
      />
      <p className="muted">
        {mode === 'login' ? (
          <>
            {m.auth.noAccount}{' '}
            <Link to="/register" search={{ next }}>
              {m.nav.register}
            </Link>
          </>
        ) : (
          <>
            {m.auth.alreadyRegistered}{' '}
            <Link to="/login" search={{ next }}>
              {m.nav.logIn}
            </Link>
          </>
        )}
      </p>
    </div>
  )
}

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: authSearch,
  beforeLoad: async ({ search }) => {
    if (await api.me()) throw redirect({ href: safeNext(search.next) })
  },
  component: function LoginPage() {
    const { next } = loginRoute.useSearch()
    return <AuthPage mode="login" next={next} />
  },
})

const registerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/register',
  validateSearch: authSearch,
  beforeLoad: async ({ search }) => {
    if (await api.me()) throw redirect({ href: safeNext(search.next) })
  },
  component: function RegisterPage() {
    const { next } = registerRoute.useSearch()
    return <AuthPage mode="register" next={next} />
  },
})

const newWorkspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/workspaces/new',
  component: function NewWorkspacePage() {
    const navigate = useNavigate()
    const router = useRouter()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.workspaces.new}</h1>
        </div>
        <WorkspaceForm
          submitting={isPending}
          errors={errors}
          cancelLink={
            <Link className="btn btn-quiet" to="/workspaces">
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { workspace } = await api.createWorkspace(input)
                void router.invalidate()
                await navigate({ to: '/w/$ws/issues', params: { ws: workspace.slug } })
              } catch (err) {
                setErrors(apiFieldErrors(err, 'slug'))
              }
            })
          }}
        />
      </div>
    )
  },
})

// --- Workspace-scoped pages (/w/{slug}/...) -------------------------------

const wsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/w/$ws',
  component: () => <Outlet />,
})

const wsIndexRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/',
  beforeLoad: ({ params }) => {
    throw redirect({ to: '/w/$ws/issues', params: { ws: params.ws } })
  },
})

const STATUS_TABS = ['open', 'closed', 'all'] as const

const issuesRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/issues',
  validateSearch: issueSearch,
  loaderDeps: ({ search }) => search,
  loader: async ({ params, deps }) => {
    const [list, labels] = await scoped(
      Promise.all([api.listIssues(params.ws, deps), api.listLabels(params.ws, { kind: 'issue' })]),
      (real) => {
        throw redirect({ to: '/w/$ws/issues', params: { ws: real }, search: deps })
      },
    )
    return { list, labels: labels.labels }
  },
  component: function IssuesPage() {
    const { list: data, labels } = issuesRoute.useLoaderData()
    const { ws } = issuesRoute.useParams()
    const search = issuesRoute.useSearch()
    const navigate = useNavigate()
    const status = search.status ?? 'open'
    const totalPages = Math.max(1, Math.ceil(data.total / data.per_page))
    const hasActiveFilter =
      search.q !== undefined ||
      search.label !== undefined ||
      (search.sort !== undefined && search.sort !== 'newest')
    const rangeStart = data.total === 0 ? 0 : (data.page - 1) * data.per_page + 1
    const rangeEnd = rangeStart + data.issues.length - 1
    const m = useMessages()
    const statusLabel = { open: m.filter.open, closed: m.filter.closed, all: m.filter.all }
    return (
      <div>
        <div className="page-head">
          <h1>{m.issues.title}</h1>
        </div>

        <nav className="tabs" aria-label={m.filter.ariaByStatus}>
          {STATUS_TABS.map((s) => (
            <Link
              key={s}
              className={`tab${status === s ? ' tab-active' : ''}`}
              to="/w/$ws/issues"
              params={{ ws }}
              search={{ ...search, status: s, page: undefined }}
              aria-current={status === s ? 'page' : undefined}
            >
              {statusLabel[s]}
            </Link>
          ))}
        </nav>

        <search>
          {/* key remounts the uncontrolled inputs when the URL filters
              change (e.g. pagination links), keeping defaults in sync */}
          <form
            key={JSON.stringify(search)}
            className="filter-bar"
            aria-label={m.filter.ariaIssues}
            onSubmit={(e) => {
              e.preventDefault()
              const fd = new FormData(e.currentTarget)
              void navigate({
                to: '/w/$ws/issues',
                params: { ws },
                search: {
                  status: str(fd.get('status')),
                  q: str(fd.get('q')),
                  label: str(fd.get('label')),
                  sort: str(fd.get('sort')),
                },
              })
            }}
          >
            <input type="hidden" name="status" value={status} />
            <div className="filter-field">
              <label htmlFor="filter-q">{m.filter.search}</label>
              <input
                type="search"
                id="filter-q"
                name="q"
                placeholder={m.filter.searchPh}
                defaultValue={search.q}
              />
            </div>
            <div className="filter-field">
              <label htmlFor="filter-label">{m.filter.label}</label>
              <select id="filter-label" name="label" defaultValue={search.label ?? ''}>
                <option value="">{m.filter.allLabels}</option>
                {labels.map((l) => (
                  <option key={l.id} value={l.name}>
                    {l.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-field">
              <label htmlFor="filter-sort">{m.filter.sort}</label>
              <select id="filter-sort" name="sort" defaultValue={search.sort ?? 'newest'}>
                <option value="newest">{m.filter.newest}</option>
                <option value="oldest">{m.filter.oldest}</option>
              </select>
            </div>
            <div className="filter-actions">
              <button type="submit" className="btn">
                {m.filter.apply}
              </button>
              {hasActiveFilter && (
                <Link className="btn btn-quiet" to="/w/$ws/issues" params={{ ws }}>
                  {m.filter.clear}
                </Link>
              )}
            </div>
          </form>
        </search>

        {data.issues.length > 0 && (
          <p className="result-count">
            {fmt(m.pager.showingIssues, { start: rangeStart, end: rangeEnd, total: data.total })}
          </p>
        )}
        <IssueList issues={data.issues} ws={ws} filtered={hasActiveFilter} search={search} />

        {totalPages > 1 && (
          <nav className="pagination" aria-label={m.pager.aria}>
            {data.page > 1 ? (
              <Link
                className="btn"
                to="/w/$ws/issues"
                params={{ ws }}
                search={{ ...search, page: data.page - 1 }}
              >
                {m.pager.prev}
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                {m.pager.prev}
              </span>
            )}
            <span className="page-info">
              {fmt(m.pager.info, { page: data.page, total: totalPages })}
            </span>
            {data.page < totalPages ? (
              <Link
                className="btn"
                to="/w/$ws/issues"
                params={{ ws }}
                search={{ ...search, page: data.page + 1 }}
              >
                {m.pager.next}
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                {m.pager.next}
              </span>
            )}
          </nav>
        )}
      </div>
    )
  },
})

// Maps a failed mutation to the per-field error map the API returns for
// 422 ({"error":{"fields":{...}}}), falling back to a generic error under
// `field`.
const apiFieldErrors = (err: unknown, field = 'title'): Record<string, string> =>
  err instanceof RequestError && err.apiError?.fields
    ? err.apiError.fields
    : { [field]: err instanceof Error ? err.message : String(err) }

const newIssueRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/issues/new',
  loader: ({ params }) =>
    scoped(api.listLabels(params.ws, { kind: 'issue' }), (real) => {
      throw redirect({ to: '/w/$ws/issues/new', params: { ws: real } })
    }),
  component: function NewIssuePage() {
    const { labels } = newIssueRoute.useLoaderData()
    const { ws } = newIssueRoute.useParams()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.issues.new}</h1>
        </div>
        <IssueForm
          submitting={isPending}
          errors={errors}
          labels={labels}
          cancelLink={
            <Link className="btn btn-quiet" to="/w/$ws/issues" params={{ ws }}>
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { issue } = await api.createIssue(ws, input)
                await navigate({
                  to: '/w/$ws/issues/$issueId',
                  params: { ws, issueId: String(issue.id) },
                })
              } catch (err) {
                setErrors(apiFieldErrors(err))
              }
            })
          }}
        />
      </div>
    )
  },
})

const editIssueRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/issues/$issueId/edit',
  loader: async ({ params }) => {
    const rebase = (real: string) => {
      throw redirect({
        to: '/w/$ws/issues/$issueId/edit',
        params: { ws: real, issueId: params.issueId },
      })
    }
    const [detail, labels] = await scoped(
      Promise.all([
        api.getIssue(params.ws, Number(params.issueId)),
        api.listLabels(params.ws, { kind: 'issue' }),
      ]),
      rebase,
    )
    return { ...detail, labels: labels.labels }
  },
  component: function EditIssuePage() {
    const { issue, labels } = editIssueRoute.useLoaderData()
    const { ws, issueId } = editIssueRoute.useParams()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{fmt(m.issues.editTitle, { id: issue.id })}</h1>
        </div>
        <IssueForm
          submitting={isPending}
          errors={errors}
          labels={labels}
          initial={{
            title: issue.title,
            body: issue.body,
            label_ids: issue.labels.map((l) => l.id),
          }}
          submitLabel={m.form.saveChanges}
          cancelLink={
            <Link className="btn btn-quiet" to="/w/$ws/issues/$issueId" params={{ ws, issueId }}>
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                await api.updateIssue(ws, Number(issueId), input)
                await navigate({
                  to: '/w/$ws/issues/$issueId',
                  params: { ws, issueId },
                })
              } catch (err) {
                setErrors(apiFieldErrors(err))
              }
            })
          }}
        />
      </div>
    )
  },
})

const issueRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/issues/$issueId',
  loader: ({ params, location }) =>
    scoped(api.getIssue(params.ws, Number(params.issueId)), (real) => {
      throw redirect({
        to: '/w/$ws/issues/$issueId',
        params: { ws: real, issueId: params.issueId },
        hash: location.hash,
      })
    }),
  component: function IssuePage() {
    const { issue, comments } = issueRoute.useLoaderData()
    const { ws, issueId } = issueRoute.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    // A reducer-driven busy flag, not an async transition: renders kicked
    // off by router.invalidate() can suspend on a loader thenable that a
    // later invalidation supersedes, which would pin isPending forever.
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [commentError, setCommentError] = useReducer(
      (_: string | null, v: string | null) => v,
      null,
    )
    const issueID = Number(issueId)
    const m = useMessages()
    const errorMessage = (err: unknown) => {
      const fields = err instanceof RequestError ? err.apiError?.fields : undefined
      return fields
        ? Object.values(fields).join(' ')
        : err instanceof RequestError
          ? err.message
          : m.error.requestFailed
    }
    const mutate = (fn: () => Promise<unknown>, onError?: (err: unknown) => void) => {
      setPending(true)
      return (async () => {
        try {
          await fn()
          void router.invalidate()
          return true
        } catch (err) {
          // A failed mutation changed nothing; refetching here can drop
          // the whole page into the route error boundary during an outage.
          onError?.(err)
          return false
        } finally {
          setPending(false)
        }
      })()
    }
    return (
      <div>
        <nav className="breadcrumb" aria-label={m.chrome.breadcrumb}>
          <ol>
            <li>
              <Link to="/w/$ws/issues" params={{ ws }} activeOptions={{ exact: true }}>
                {m.issues.title}
              </Link>
            </li>
            <li>
              <a href="#" aria-current="page">
                #{issue.id}
              </a>
            </li>
          </ol>
        </nav>

        <IssueDetail
          issue={issue}
          ws={ws}
          pending={isPending}
          onSetStatus={(status) => mutate(() => api.setIssueStatus(ws, issueID, status))}
          onDelete={() => {
            setPending(true)
            void (async () => {
              try {
                await api.deleteIssue(ws, issueID)
                // The page is gone — a dirty comment draft must not
                // trap the post-delete redirect in the leave-guard.
                await navigate({ to: '/w/$ws/issues', params: { ws }, ignoreBlocker: true })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getIssueMarkdown(ws, issueID)}
        />

        <section id="comments" className="comments">
          <h2>
            {m.issues.comments}
            {comments.length > 0 && <span className="comment-count"> ({comments.length})</span>}
          </h2>
          <CommentList
            comments={comments}
            pending={isPending}
            onEdit={(cid, body) => mutate(() => api.updateComment(ws, issueID, cid, body))}
            onDelete={(cid) => mutate(() => api.deleteComment(ws, issueID, cid))}
            onHide={(cid, reason) => mutate(() => api.hideComment(ws, issueID, cid, reason))}
            onUnhide={(cid) => mutate(() => api.unhideComment(ws, issueID, cid))}
          />
          <CommentForm
            submitting={isPending}
            error={commentError}
            onSubmit={(body) => {
              setCommentError(null)
              return mutate(
                () => api.createComment(ws, issueID, body),
                (err) => setCommentError(errorMessage(err)),
              )
            }}
          />
        </section>

        <a className="back-to-top" href="#main-content" aria-label={m.chrome.backToTopAria}>
          {m.chrome.backToTop}
        </a>
      </div>
    )
  },
})

const discussionsRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/discussions',
  validateSearch: discussionSearch,
  loaderDeps: ({ search }) => search,
  loader: async ({ params, deps }) => {
    const [list, categories] = await scoped(
      Promise.all([api.listDiscussions(params.ws, deps), api.listDiscussionCategories()]),
      (real) => {
        throw redirect({ to: '/w/$ws/discussions', params: { ws: real }, search: deps })
      },
    )
    return { list, categories: categories.categories }
  },
  component: function DiscussionsPage() {
    const { list: data, categories } = discussionsRoute.useLoaderData()
    const { ws } = discussionsRoute.useParams()
    const search = discussionsRoute.useSearch()
    const navigate = useNavigate()
    const totalPages = Math.max(1, Math.ceil(data.total / data.per_page))
    const hasActiveFilter =
      search.category !== undefined ||
      search.answered !== undefined ||
      search.q !== undefined ||
      (search.sort !== undefined && search.sort !== 'newest')
    const rangeStart = data.total === 0 ? 0 : (data.page - 1) * data.per_page + 1
    const rangeEnd = rangeStart + data.discussions.length - 1
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.discussions.title}</h1>
          <Link className="btn" to="/w/$ws/discussions/new" params={{ ws }}>
            {m.discussions.new}
          </Link>
        </div>

        <search>
          {/* key remounts the uncontrolled inputs when the URL filters change */}
          <form
            key={JSON.stringify(search)}
            className="filter-bar"
            aria-label={m.filter.ariaDiscussions}
            onSubmit={(e) => {
              e.preventDefault()
              const fd = new FormData(e.currentTarget)
              void navigate({
                to: '/w/$ws/discussions',
                params: { ws },
                search: {
                  category: str(fd.get('category')),
                  answered: str(fd.get('answered')),
                  q: str(fd.get('q')),
                  sort: str(fd.get('sort')),
                },
              })
            }}
          >
            <div className="filter-field">
              <label htmlFor="filter-category">{m.filter.category}</label>
              <select id="filter-category" name="category" defaultValue={search.category ?? ''}>
                <option value="">{m.filter.allCategories}</option>
                {categories.map((c) => (
                  <option key={c.id} value={c.slug}>
                    {c.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-field">
              <label htmlFor="filter-answered">{m.filter.answered}</label>
              <select id="filter-answered" name="answered" defaultValue={search.answered ?? ''}>
                <option value="">{m.filter.all}</option>
                <option value="yes">{m.filter.answered}</option>
                <option value="no">{m.filter.unanswered}</option>
              </select>
            </div>
            <div className="filter-field">
              <label htmlFor="filter-dq">{m.filter.search}</label>
              <input
                type="search"
                id="filter-dq"
                name="q"
                placeholder={m.filter.searchPh}
                defaultValue={search.q}
              />
            </div>
            <div className="filter-field">
              <label htmlFor="filter-dsort">{m.filter.sort}</label>
              <select id="filter-dsort" name="sort" defaultValue={search.sort ?? 'newest'}>
                <option value="newest">{m.filter.newest}</option>
                <option value="oldest">{m.filter.oldest}</option>
              </select>
            </div>
            <div className="filter-actions">
              <button type="submit" className="btn">
                {m.filter.apply}
              </button>
              {hasActiveFilter && (
                <Link className="btn btn-quiet" to="/w/$ws/discussions" params={{ ws }}>
                  {m.filter.clear}
                </Link>
              )}
            </div>
          </form>
        </search>

        {data.discussions.length > 0 && (
          <p className="result-count">
            {fmt(m.pager.showingDiscussions, {
              start: rangeStart,
              end: rangeEnd,
              total: data.total,
            })}
          </p>
        )}
        <DiscussionList discussions={data.discussions} ws={ws} filtered={hasActiveFilter} />

        {totalPages > 1 && (
          <nav className="pagination" aria-label={m.pager.aria}>
            {data.page > 1 ? (
              <Link
                className="btn"
                to="/w/$ws/discussions"
                params={{ ws }}
                search={{ ...search, page: data.page - 1 }}
              >
                {m.pager.prev}
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                {m.pager.prev}
              </span>
            )}
            <span className="page-info">
              {fmt(m.pager.info, { page: data.page, total: totalPages })}
            </span>
            {data.page < totalPages ? (
              <Link
                className="btn"
                to="/w/$ws/discussions"
                params={{ ws }}
                search={{ ...search, page: data.page + 1 }}
              >
                {m.pager.next}
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                {m.pager.next}
              </span>
            )}
          </nav>
        )}
      </div>
    )
  },
})

const newDiscussionRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/discussions/new',
  loader: async ({ params }) => {
    const [categories, labels] = await scoped(
      Promise.all([
        api.listDiscussionCategories(),
        api.listLabels(params.ws, { kind: 'discussion' }),
      ]),
      (real) => {
        throw redirect({ to: '/w/$ws/discussions/new', params: { ws: real } })
      },
    )
    return { categories: categories.categories, labels: labels.labels }
  },
  component: function NewDiscussionPage() {
    const { categories, labels } = newDiscussionRoute.useLoaderData()
    const { ws } = newDiscussionRoute.useParams()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.discussions.new}</h1>
        </div>
        <DiscussionForm
          submitting={isPending}
          errors={errors}
          categories={categories}
          labels={labels}
          cancelLink={
            <Link className="btn btn-quiet" to="/w/$ws/discussions" params={{ ws }}>
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { discussion } = await api.createDiscussion(ws, input)
                await navigate({
                  to: '/w/$ws/discussions/$discussionId',
                  params: { ws, discussionId: String(discussion.id) },
                })
              } catch (err) {
                setErrors(apiFieldErrors(err, 'category'))
              }
            })
          }}
        />
      </div>
    )
  },
})

const editDiscussionRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/discussions/$discussionId/edit',
  loader: async ({ params }) => {
    const [detail, labels, categories] = await scoped(
      Promise.all([
        api.getDiscussion(params.ws, Number(params.discussionId)),
        api.listLabels(params.ws, { kind: 'discussion' }),
        api.listDiscussionCategories(),
      ]),
      (real) => {
        throw redirect({
          to: '/w/$ws/discussions/$discussionId/edit',
          params: { ws: real, discussionId: params.discussionId },
        })
      },
    )
    return { ...detail, labels: labels.labels, categories: categories.categories }
  },
  component: function EditDiscussionPage() {
    const { discussion, labels, categories } = editDiscussionRoute.useLoaderData()
    const { ws, discussionId } = editDiscussionRoute.useParams()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{fmt(m.discussions.editTitle, { id: discussion.id })}</h1>
        </div>
        <DiscussionForm
          submitting={isPending}
          errors={errors}
          categories={categories}
          labels={labels}
          initial={{
            title: discussion.title,
            body: discussion.body,
            category_id: discussion.category.id,
            label_ids: discussion.labels.map((l) => l.id),
          }}
          submitLabel={m.form.saveChanges}
          cancelLink={
            <Link
              className="btn btn-quiet"
              to="/w/$ws/discussions/$discussionId"
              params={{ ws, discussionId }}
            >
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                await api.updateDiscussion(ws, Number(discussionId), input)
                await navigate({
                  to: '/w/$ws/discussions/$discussionId',
                  params: { ws, discussionId },
                })
              } catch (err) {
                setErrors(apiFieldErrors(err, 'category'))
              }
            })
          }}
        />
      </div>
    )
  },
})

const discussionRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/discussions/$discussionId',
  loader: ({ params, location }) =>
    scoped(api.getDiscussion(params.ws, Number(params.discussionId)), (real) => {
      throw redirect({
        to: '/w/$ws/discussions/$discussionId',
        params: { ws: real, discussionId: params.discussionId },
        hash: location.hash,
      })
    }),
  component: function DiscussionPage() {
    const { discussion, threads } = discussionRoute.useLoaderData()
    const { ws, discussionId } = discussionRoute.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    // A reducer-driven busy flag, not an async transition: renders kicked
    // off by router.invalidate() can suspend on a loader thenable that a
    // later invalidation supersedes, which would pin isPending forever.
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const did = Number(discussionId)
    const m = useMessages()
    const [error, setError] = useReducer((_: string | null, v: string | null) => v, null)
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      return (async () => {
        try {
          await fn()
          setError(null)
          void router.invalidate()
          return true
        } catch (err) {
          const fields = err instanceof RequestError ? err.apiError?.fields : undefined
          // Field errors name the actual rejection reason (e.g. a
          // whitespace-only reply); the bare message just says
          // "Validation failed.".
          setError(
            fields
              ? Object.values(fields).join(' ')
              : err instanceof RequestError
                ? err.message
                : m.error.requestFailed,
          )
          return false
        } finally {
          setPending(false)
        }
      })()
    }
    // Posting a thread or reply lands on the new entry's permalink like
    // the MPA's redirect-to-anchor flow.
    const postAndGo = <T,>(fn: () => Promise<T>, anchor: (r: T) => string): Promise<boolean> => {
      setPending(true)
      return (async () => {
        try {
          const created = await fn()
          await router.invalidate()
          void navigate({ to: '.', hash: anchor(created), replace: true })
          // The reloaded list renders after invalidate resolves, so the
          // router's hash scroll may run before the entry exists — retry
          // on the next frame.
          requestAnimationFrame(() => {
            document.getElementById(anchor(created))?.scrollIntoView()
          })
          return true
        } catch {
          // mutation failures surface via the refetched state
          return false
        } finally {
          setPending(false)
        }
      })()
    }
    const answerable = discussion.category.format === 'qa'
    return (
      <div>
        <nav className="breadcrumb" aria-label={m.chrome.breadcrumb}>
          <ol>
            <li>
              <Link to="/w/$ws/discussions" params={{ ws }} activeOptions={{ exact: true }}>
                {m.discussions.title}
              </Link>
            </li>
            <li>
              <a href="#" aria-current="page">
                #{discussion.id}
              </a>
            </li>
          </ol>
        </nav>

        <DiscussionDetail
          discussion={discussion}
          ws={ws}
          pending={isPending}
          onDelete={() => {
            setPending(true)
            void (async () => {
              try {
                await api.deleteDiscussion(ws, did)
                await navigate({
                  to: '/w/$ws/discussions',
                  params: { ws },
                  ignoreBlocker: true,
                })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getDiscussionMarkdown(ws, did)}
        />

        <section id="comments" className="comments">
          <h2>
            {m.issues.comments}
            {threads.length > 0 && <span className="comment-count"> ({threads.length})</span>}
          </h2>
          <ThreadList
            threads={threads}
            pending={isPending}
            answerable={answerable}
            onEdit={(tid, body) => mutate(() => api.updateThread(ws, did, tid, body))}
            onDelete={(tid) => mutate(() => api.deleteThread(ws, did, tid))}
            onHide={(tid, reason) => mutate(() => api.hideThread(ws, did, tid, reason))}
            onUnhide={(tid) => mutate(() => api.unhideThread(ws, did, tid))}
            onSetAnswer={
              answerable
                ? (tid, answer) => mutate(() => api.setThreadAnswer(ws, did, tid, answer))
                : undefined
            }
            onReply={(tid, body) =>
              postAndGo(
                () => api.createReply(ws, did, tid, body),
                (r) => `comment-${r.reply.id}`,
              )
            }
            replyHandlers={{
              pending: isPending,
              onEdit: (tid, cid, body) => mutate(() => api.updateReply(ws, did, tid, cid, body)),
              onDelete: (tid, cid) => mutate(() => api.deleteReply(ws, did, tid, cid)),
              onHide: (tid, cid, reason) => mutate(() => api.hideReply(ws, did, tid, cid, reason)),
              onUnhide: (tid, cid) => mutate(() => api.unhideReply(ws, did, tid, cid)),
            }}
          />
          {error && <em role="alert">{error}</em>}
          <CommentForm
            submitting={isPending}
            onSubmit={(body) =>
              postAndGo(
                () => api.createThread(ws, did, body),
                (r) => `thread-${r.thread.id}`,
              )
            }
          />
        </section>

        <a className="back-to-top" href="#main-content" aria-label={m.chrome.backToTopAria}>
          {m.chrome.backToTop}
        </a>
      </div>
    )
  },
})

// --- Pull requests (workspace-independent) ---------------------------------
// Mount only when the server resolved a git repo; a 404 on /api/v1/pulls
// means the feature is off, so loaders surface it as a plain notFound.

type PullSearch = {
  status?: string | undefined
  q?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

const pullSearch = (search: Record<string, unknown>): PullSearch => ({
  status: str(search.status),
  q: str(search.q),
  sort: str(search.sort),
  page: num(search.page),
})

const pullOrNotFound = async <T,>(p: Promise<T>): Promise<T> => {
  try {
    return await p
  } catch (err) {
    if (err instanceof RequestError && err.status === 404) throw notFound()
    throw err
  }
}

const pullsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pulls',
  validateSearch: pullSearch,
  loaderDeps: ({ search }) => search,
  loader: ({ deps }) => pullOrNotFound(api.listPulls(deps)),
  component: function PullsPage() {
    const data = pullsRoute.useLoaderData()
    const search = pullsRoute.useSearch()
    const navigate = useNavigate()
    const totalPages = Math.max(1, Math.ceil(data.total / data.per_page))
    const hasActiveFilter =
      (search.status !== undefined && search.status !== 'open') ||
      search.q !== undefined ||
      (search.sort !== undefined && search.sort !== 'newest')
    const rangeStart = data.total === 0 ? 0 : (data.page - 1) * data.per_page + 1
    const rangeEnd = rangeStart + data.pulls.length - 1
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.pulls.title}</h1>
          <Link className="btn" to="/pulls/new">
            {m.pulls.new}
          </Link>
        </div>

        <search>
          {/* key remounts the uncontrolled inputs when the URL filters change */}
          <form
            key={JSON.stringify(search)}
            className="filter-bar"
            aria-label={m.filter.ariaPulls}
            onSubmit={(e) => {
              e.preventDefault()
              const fd = new FormData(e.currentTarget)
              void navigate({
                to: '/pulls',
                search: {
                  status: str(fd.get('status')),
                  q: str(fd.get('q')),
                  sort: str(fd.get('sort')),
                },
              })
            }}
          >
            <div className="filter-field">
              <label htmlFor="filter-pstatus">{m.filter.status}</label>
              <select id="filter-pstatus" name="status" defaultValue={search.status ?? 'open'}>
                <option value="open">{m.filter.open}</option>
                <option value="closed">{m.filter.closed}</option>
                <option value="merged">{m.filter.merged}</option>
                <option value="all">{m.filter.all}</option>
              </select>
            </div>
            <div className="filter-field">
              <label htmlFor="filter-pq">{m.filter.search}</label>
              <input
                type="search"
                id="filter-pq"
                name="q"
                placeholder={m.filter.searchPh}
                defaultValue={search.q}
              />
            </div>
            <div className="filter-field">
              <label htmlFor="filter-psort">{m.filter.sort}</label>
              <select id="filter-psort" name="sort" defaultValue={search.sort ?? 'newest'}>
                <option value="newest">{m.filter.newest}</option>
                <option value="oldest">{m.filter.oldest}</option>
              </select>
            </div>
            <div className="filter-actions">
              <button type="submit" className="btn">
                {m.filter.apply}
              </button>
              {hasActiveFilter && (
                <Link className="btn btn-quiet" to="/pulls">
                  {m.filter.clear}
                </Link>
              )}
            </div>
          </form>
        </search>

        {data.pulls.length > 0 && (
          <p className="result-count">
            {fmt(m.pager.showingPulls, { start: rangeStart, end: rangeEnd, total: data.total })}
          </p>
        )}
        <PullList pulls={data.pulls} filtered={hasActiveFilter} />

        {totalPages > 1 && (
          <nav className="pagination" aria-label={m.pager.aria}>
            {data.page > 1 ? (
              <Link className="btn" to="/pulls" search={{ ...search, page: data.page - 1 }}>
                {m.pager.prev}
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                {m.pager.prev}
              </span>
            )}
            <span className="page-info">
              {fmt(m.pager.info, { page: data.page, total: totalPages })}
            </span>
            {data.page < totalPages ? (
              <Link className="btn" to="/pulls" search={{ ...search, page: data.page + 1 }}>
                {m.pager.next}
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                {m.pager.next}
              </span>
            )}
          </nav>
        )}
      </div>
    )
  },
})

const newPullRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pulls/new',
  loader: () => pullOrNotFound(api.listPullRefs()),
  component: function NewPullPage() {
    const { refs } = newPullRoute.useLoaderData()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.pulls.new}</h1>
        </div>
        <PullForm
          submitting={isPending}
          errors={errors}
          refs={refs}
          cancelLink={
            <Link className="btn btn-quiet" to="/pulls">
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { pull_request } = await api.createPull({
                  title: input.title,
                  body: input.body,
                  base: input.base ?? '',
                  head: input.head ?? '',
                })
                await navigate({
                  to: '/pulls/$pullId',
                  params: { pullId: String(pull_request.id) },
                })
              } catch (err) {
                setErrors(apiFieldErrors(err, 'head'))
              }
            })
          }}
        />
      </div>
    )
  },
})

const editPullRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pulls/$pullId/edit',
  loader: ({ params }) => pullOrNotFound(api.getPull(Number(params.pullId))),
  component: function EditPullPage() {
    const { pull_request } = editPullRoute.useLoaderData()
    const { pullId } = editPullRoute.useParams()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{fmt(m.pulls.editTitle, { id: pull_request.id })}</h1>
        </div>
        <PullForm
          submitting={isPending}
          errors={errors}
          initial={{ title: pull_request.title, body: pull_request.body }}
          submitLabel={m.form.saveChanges}
          cancelLink={
            <Link preload={false} className="btn btn-quiet" to="/pulls/$pullId" params={{ pullId }}>
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                await api.updatePull(Number(pullId), {
                  title: input.title,
                  body: input.body,
                })
                await navigate({ to: '/pulls/$pullId', params: { pullId } })
              } catch (err) {
                setErrors(apiFieldErrors(err))
              }
            })
          }}
        />
      </div>
    )
  },
})

const pullFilesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pulls/$pullId/files',
  loader: ({ params }) =>
    pullOrNotFound(
      Promise.all([
        api.getPull(Number(params.pullId)),
        api.getPullDiff(Number(params.pullId)),
      ]).then(([detail, diff]) => ({ ...detail, diff })),
    ),
  component: function PullFilesPage() {
    const { pull_request, diff } = pullFilesRoute.useLoaderData()
    const { text: diffText, head_sha: headSha, truncated } = diff
    const { pullId } = pullFilesRoute.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const m = useMessages()
    return (
      <div>
        <nav className="breadcrumb" aria-label={m.chrome.breadcrumb}>
          <ol>
            <li>
              <Link to="/pulls" activeOptions={{ exact: true }}>
                {m.pulls.title}
              </Link>
            </li>
            <li>
              <Link preload={false} to="/pulls/$pullId" params={{ pullId }}>
                #{pull_request.id}
              </Link>
            </li>
            <li>
              <a href="#" aria-current="page">
                {m.pulls.files}
              </a>
            </li>
          </ol>
        </nav>

        <div className="page-head">
          <h1>
            {pull_request.title} <span className="issue-num">#{pull_request.id}</span>
          </h1>
          <Link preload={false} className="btn" to="/pulls/$pullId" params={{ pullId }}>
            {m.pulls.conversation}
          </Link>
        </div>

        {truncated && (
          <p className="notice" role="status">
            {m.pulls.truncated}
          </p>
        )}
        <PullFiles
          diff={diffText}
          pending={isPending}
          onComment={(input: PRCommentInput) => {
            setPending(true)
            return (async () => {
              try {
                const { comment } = await api.createPRComment(Number(pullId), {
                  ...input,
                  anchor: headSha,
                })
                // Invalidate before navigating so the destination never
                // serves the pre-mutation loader snapshot (ui-rules §13).
                void router.invalidate()
                await navigate({
                  to: '/pulls/$pullId',
                  params: { pullId },
                  hash: `comment-${comment.id}`,
                })
              } catch (err) {
                // Stale anchor/path errors need a fresh diff. Network/5xx
                // failures must not refetch: the loader failure would
                // replace the page — and the preserved draft — with the
                // route error boundary.
                if (
                  err instanceof RequestError &&
                  (err.apiError?.fields?.anchor !== undefined ||
                    err.apiError?.fields?.path !== undefined)
                ) {
                  void router.invalidate()
                }
                throw err
              } finally {
                setPending(false)
              }
            })()
          }}
        />
      </div>
    )
  },
})

const pullRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pulls/$pullId',
  loader: ({ params }) => pullOrNotFound(api.getPull(Number(params.pullId))),
  component: function PullPage() {
    const { pull_request, comments, commits, mergeable } = pullRoute.useLoaderData()
    const { pullId } = pullRoute.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [actionError, setActionError] = useReducer(
      (_: string | null, v: string | null) => v,
      null,
    )
    const pid = Number(pullId)
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      setActionError(null)
      return (async () => {
        try {
          await fn()
          void router.invalidate()
          return true
        } catch (err) {
          // A failed mutation changes nothing; refetching here can drop
          // the whole page into the route error boundary during an outage.
          setActionError(err instanceof RequestError ? err.message : m.error.requestFailed)
          return false
        } finally {
          setPending(false)
        }
      })()
    }
    const m = useMessages()
    return (
      <div>
        <nav className="breadcrumb" aria-label={m.chrome.breadcrumb}>
          <ol>
            <li>
              <Link to="/pulls" activeOptions={{ exact: true }}>
                {m.pulls.title}
              </Link>
            </li>
            <li>
              <a href="#" aria-current="page">
                #{pull_request.id}
              </a>
            </li>
          </ol>
        </nav>

        <PullDetail
          pull={pull_request}
          mergeable={mergeable}
          pending={isPending}
          actionError={actionError}
          onSetStatus={(status) => mutate(() => api.setPullStatus(pid, status))}
          onMerge={(method, message) =>
            mutate(() => api.mergePull(pid, method, message || undefined))
          }
          onDelete={() => {
            setPending(true)
            void (async () => {
              try {
                await api.deletePull(pid)
                await navigate({ to: '/pulls', ignoreBlocker: true })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getPullMarkdown(pid)}
        />

        <PullCommitList commits={commits} />

        <section id="comments" className="comments">
          <h2>
            {m.issues.comments}
            {comments.length > 0 && <span className="comment-count"> ({comments.length})</span>}
          </h2>
          <PRCommentList
            comments={comments}
            pending={isPending}
            onEdit={(cid, body) => mutate(() => api.updatePRComment(pid, cid, body))}
            onDelete={(cid) => mutate(() => api.deletePRComment(pid, cid))}
            onHide={(cid, reason) => mutate(() => api.hidePRComment(pid, cid, reason))}
            onUnhide={(cid) => mutate(() => api.unhidePRComment(pid, cid))}
          />
          <CommentForm
            submitting={isPending}
            onSubmit={(body) => mutate(() => api.createPRComment(pid, { body }))}
          />
        </section>

        <a className="back-to-top" href="#main-content" aria-label={m.chrome.backToTopAria}>
          {m.chrome.backToTop}
        </a>
      </div>
    )
  },
})

const labelsRoute = createRoute({
  getParentRoute: () => wsRoute,
  path: '/labels',
  loader: async ({ params }) => {
    const [active, archived] = await scoped(
      Promise.all([api.listLabels(params.ws), api.listLabels(params.ws, { archived: true })]),
      (real) => {
        throw redirect({ to: '/w/$ws/labels', params: { ws: real } })
      },
    )
    return { active: active.labels, archived: archived.labels }
  },
  component: function LabelsPage() {
    const { active, archived } = labelsRoute.useLoaderData()
    const { ws } = labelsRoute.useParams()
    const router = useRouter()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      void (async () => {
        try {
          await fn()
          setErrors({})
          void router.invalidate()
        } catch (err) {
          setErrors(apiFieldErrors(err))
        } finally {
          setPending(false)
        }
      })()
    }
    return (
      <LabelsAdmin
        active={active}
        archived={archived}
        pending={isPending}
        errors={errors}
        onCreate={(input) => mutate(() => api.createLabel(ws, input))}
        onUpdate={(id, patch) => mutate(() => api.updateLabel(ws, id, patch))}
        onArchive={(id) => mutate(() => api.archiveLabel(ws, id))}
        onRestore={(l) =>
          mutate(() =>
            api.createLabel(ws, {
              name: l.name,
              color: l.color,
              kind: l.kind,
              shared: l.workspace_id == null,
            }),
          )
        }
      />
    )
  },
})

export const routeTree = rootRoute.addChildren([
  indexRoute,
  issuesRedirectRoute,
  newIssueRedirectRoute,
  issueRedirectRoute,
  editIssueRedirectRoute,
  discussionsRedirectRoute,
  newDiscussionRedirectRoute,
  discussionRedirectRoute,
  editDiscussionRedirectRoute,
  labelsRedirectRoute,
  workspacesRoute,
  newWorkspaceRoute,
  loginRoute,
  registerRoute,
  pullsRoute,
  newPullRoute,
  pullRoute,
  editPullRoute,
  pullFilesRoute,
  wsRoute.addChildren([
    wsIndexRoute,
    issuesRoute,
    newIssueRoute,
    issueRoute,
    editIssueRoute,
    discussionsRoute,
    newDiscussionRoute,
    discussionRoute,
    editDiscussionRoute,
    labelsRoute,
  ]),
])
