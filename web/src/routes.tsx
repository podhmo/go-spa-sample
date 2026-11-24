import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  isNotFound,
  notFound,
  redirect,
  useNavigate,
  useParams,
  useRouter,
} from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { RequestError, api } from './api'
import { CommentForm, DiscussionForm, IssueForm, WorkspaceForm } from './components/forms'
import { CommentList, IssueDetail } from './components/IssueDetail'
import { IssueList } from './components/IssueList'
import { DiscussionList } from './components/DiscussionList'
import { DiscussionDetail, ThreadList } from './components/DiscussionDetail'
import { LabelsAdmin } from './components/LabelsAdmin'
import { WorkspaceList } from './components/Workspaces'

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
function SiteNav({ ws }: { ws: string | undefined }) {
  if (ws !== undefined) {
    return (
      <>
        <Link className="site-title" to="/w/$ws/issues" params={{ ws }}>
          zatudan
        </Link>
        <nav className="site-nav" aria-label="Main">
          <Link to="/w/$ws/issues" params={{ ws }}>
            Issues
          </Link>
          <Link to="/w/$ws/discussions" params={{ ws }}>
            Discussions
          </Link>
          <Link to="/w/$ws/labels" params={{ ws }}>
            Labels
          </Link>
          <Link to="/workspaces">Workspaces</Link>
          <Link className="btn" to="/w/$ws/issues/new" params={{ ws }}>
            New issue
          </Link>
        </nav>
      </>
    )
  }
  return (
    <>
      <Link className="site-title" to="/">
        zatudan
      </Link>
      <nav className="site-nav" aria-label="Main">
        <Link to="/issues">Issues</Link>
        <Link to="/discussions">Discussions</Link>
        <Link to="/labels">Labels</Link>
        <Link to="/workspaces">Workspaces</Link>
        <Link className="btn" to="/issues/new">
          New issue
        </Link>
      </nav>
    </>
  )
}

const rootRoute = createRootRoute({
  component: function RootLayout() {
    const { ws } = useParams({ strict: false })
    return (
      <>
        <a className="skip-link" href="#main-content">
          Skip to content
        </a>
        <header className="site-header">
          <div className="header-bar">
            <div className="container header-inner">
              <SiteNav ws={ws} />
            </div>
          </div>
        </header>
        <main className="container main-content" id="main-content" tabIndex={-1}>
          <Outlet />
        </main>
        <footer className="site-footer">
          <div className="container">
            <p>
              A single-page app port of zatudan — Go API + React, TanStack Router, minimal
              JavaScript where it earns its place.
            </p>
          </div>
        </footer>
      </>
    )
  },
  notFoundComponent: () => (
    <div className="error-page">
      <p className="error-code">404</p>
      <h1>404 — not found</h1>
      <p>
        The page or resource you are looking for does not exist.{' '}
        <Link to="/issues">Back to issues</Link>
      </p>
    </div>
  ),
  errorComponent: ({ error }) => (
    <div className="error-page">
      <h1>Something went wrong</h1>
      {isNotFound(error) ? (
        <p>
          The page or resource you are looking for does not exist.{' '}
          <Link to="/issues">Back to issues</Link>
        </p>
      ) : (
        <p role="alert">{error instanceof Error ? error.message : String(error)}</p>
      )}
    </div>
  ),
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

const str = (v: unknown) => (typeof v === 'string' && v !== '' ? v : undefined)
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
        } catch (err) {
          setError(err instanceof Error ? err.message : String(err))
        } finally {
          setPending(false)
          void router.invalidate()
        }
      })()
    }
    return (
      <div>
        <div className="page-head">
          <h1>Workspaces</h1>
          <Link className="btn" to="/workspaces/new">
            New workspace
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
    return (
      <div>
        <div className="page-head">
          <h1>New workspace</h1>
        </div>
        <WorkspaceForm
          submitting={isPending}
          errors={errors}
          cancelLink={
            <Link className="btn btn-quiet" to="/workspaces">
              Cancel
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
    return (
      <div>
        <div className="page-head">
          <h1>Issues</h1>
        </div>

        <nav className="tabs" aria-label="Filter by status">
          {STATUS_TABS.map((s) => (
            <Link
              key={s}
              className={`tab${status === s ? ' tab-active' : ''}`}
              to="/w/$ws/issues"
              params={{ ws }}
              search={{ ...search, status: s, page: undefined }}
              aria-current={status === s ? 'page' : undefined}
            >
              {s.charAt(0).toUpperCase() + s.slice(1)}
            </Link>
          ))}
        </nav>

        <search>
          {/* key remounts the uncontrolled inputs when the URL filters
              change (e.g. pagination links), keeping defaults in sync */}
          <form
            key={JSON.stringify(search)}
            className="filter-bar"
            aria-label="Filter issues"
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
              <label htmlFor="filter-q">Search</label>
              <input
                type="search"
                id="filter-q"
                name="q"
                placeholder="Title or body"
                defaultValue={search.q}
              />
            </div>
            <div className="filter-field">
              <label htmlFor="filter-label">Label</label>
              <select id="filter-label" name="label" defaultValue={search.label ?? ''}>
                <option value="">All labels</option>
                {labels.map((l) => (
                  <option key={l.id} value={l.name}>
                    {l.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-field">
              <label htmlFor="filter-sort">Sort</label>
              <select id="filter-sort" name="sort" defaultValue={search.sort ?? 'newest'}>
                <option value="newest">Newest first</option>
                <option value="oldest">Oldest first</option>
              </select>
            </div>
            <div className="filter-actions">
              <button type="submit" className="btn">
                Apply
              </button>
              {hasActiveFilter && (
                <Link className="btn btn-quiet" to="/w/$ws/issues" params={{ ws }}>
                  Clear
                </Link>
              )}
            </div>
          </form>
        </search>

        {data.issues.length > 0 && (
          <p className="result-count">
            Showing {rangeStart}–{rangeEnd} of {data.total} issues
          </p>
        )}
        <IssueList issues={data.issues} ws={ws} filtered={hasActiveFilter} search={search} />

        {totalPages > 1 && (
          <nav className="pagination" aria-label="Pagination">
            {data.page > 1 ? (
              <Link
                className="btn"
                to="/w/$ws/issues"
                params={{ ws }}
                search={{ ...search, page: data.page - 1 }}
              >
                Previous
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                Previous
              </span>
            )}
            <span className="page-info">
              Page {data.page} of {totalPages}
            </span>
            {data.page < totalPages ? (
              <Link
                className="btn"
                to="/w/$ws/issues"
                params={{ ws }}
                search={{ ...search, page: data.page + 1 }}
              >
                Next
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                Next
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
    return (
      <div>
        <div className="page-head">
          <h1>New issue</h1>
        </div>
        <IssueForm
          submitting={isPending}
          errors={errors}
          labels={labels}
          cancelLink={
            <Link className="btn btn-quiet" to="/w/$ws/issues" params={{ ws }}>
              Cancel
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
    return (
      <div>
        <div className="page-head">
          <h1>Edit issue #{issue.id}</h1>
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
          submitLabel="Save changes"
          cancelLink={
            <Link className="btn btn-quiet" to="/w/$ws/issues/$issueId" params={{ ws, issueId }}>
              Cancel
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
    const issueID = Number(issueId)
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      void (async () => {
        try {
          await fn()
        } catch {
          // mutation failures surface via the refetched state
        } finally {
          setPending(false)
          void router.invalidate()
        }
      })()
    }
    return (
      <div>
        <nav className="breadcrumb" aria-label="Breadcrumb">
          <ol>
            <li>
              <Link to="/w/$ws/issues" params={{ ws }} activeOptions={{ exact: true }}>
                Issues
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
                await navigate({ to: '/w/$ws/issues', params: { ws } })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getIssueMarkdown(ws, issueID)}
        />

        <section id="comments" className="comments">
          <h2>
            Comments
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
            onSubmit={(body) => mutate(() => api.createComment(ws, issueID, body))}
          />
        </section>

        <a className="back-to-top" href="#main-content" aria-label="Back to top">
          ↑ Top
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
    return (
      <div>
        <div className="page-head">
          <h1>Discussions</h1>
          <Link className="btn" to="/w/$ws/discussions/new" params={{ ws }}>
            New discussion
          </Link>
        </div>

        <search>
          {/* key remounts the uncontrolled inputs when the URL filters change */}
          <form
            key={JSON.stringify(search)}
            className="filter-bar"
            aria-label="Filter discussions"
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
              <label htmlFor="filter-category">Category</label>
              <select id="filter-category" name="category" defaultValue={search.category ?? ''}>
                <option value="">All categories</option>
                {categories.map((c) => (
                  <option key={c.id} value={c.slug}>
                    {c.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="filter-field">
              <label htmlFor="filter-answered">Answered</label>
              <select id="filter-answered" name="answered" defaultValue={search.answered ?? ''}>
                <option value="">All</option>
                <option value="yes">Answered</option>
                <option value="no">Unanswered</option>
              </select>
            </div>
            <div className="filter-field">
              <label htmlFor="filter-dq">Search</label>
              <input
                type="search"
                id="filter-dq"
                name="q"
                placeholder="Title or body"
                defaultValue={search.q}
              />
            </div>
            <div className="filter-field">
              <label htmlFor="filter-dsort">Sort</label>
              <select id="filter-dsort" name="sort" defaultValue={search.sort ?? 'newest'}>
                <option value="newest">Newest first</option>
                <option value="oldest">Oldest first</option>
              </select>
            </div>
            <div className="filter-actions">
              <button type="submit" className="btn">
                Apply
              </button>
              {hasActiveFilter && (
                <Link className="btn btn-quiet" to="/w/$ws/discussions" params={{ ws }}>
                  Clear
                </Link>
              )}
            </div>
          </form>
        </search>

        {data.discussions.length > 0 && (
          <p className="result-count">
            Showing {rangeStart}–{rangeEnd} of {data.total} discussions
          </p>
        )}
        <DiscussionList discussions={data.discussions} ws={ws} filtered={hasActiveFilter} />

        {totalPages > 1 && (
          <nav className="pagination" aria-label="Pagination">
            {data.page > 1 ? (
              <Link
                className="btn"
                to="/w/$ws/discussions"
                params={{ ws }}
                search={{ ...search, page: data.page - 1 }}
              >
                Previous
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                Previous
              </span>
            )}
            <span className="page-info">
              Page {data.page} of {totalPages}
            </span>
            {data.page < totalPages ? (
              <Link
                className="btn"
                to="/w/$ws/discussions"
                params={{ ws }}
                search={{ ...search, page: data.page + 1 }}
              >
                Next
              </Link>
            ) : (
              <span className="btn btn-disabled" aria-disabled="true">
                Next
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
    return (
      <div>
        <div className="page-head">
          <h1>New discussion</h1>
        </div>
        <DiscussionForm
          submitting={isPending}
          errors={errors}
          categories={categories}
          labels={labels}
          cancelLink={
            <Link className="btn btn-quiet" to="/w/$ws/discussions" params={{ ws }}>
              Cancel
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
    return (
      <div>
        <div className="page-head">
          <h1>Edit discussion #{discussion.id}</h1>
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
          submitLabel="Save changes"
          cancelLink={
            <Link
              className="btn btn-quiet"
              to="/w/$ws/discussions/$discussionId"
              params={{ ws, discussionId }}
            >
              Cancel
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
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      void (async () => {
        try {
          await fn()
        } catch {
          // mutation failures surface via the refetched state
        } finally {
          setPending(false)
          void router.invalidate()
        }
      })()
    }
    const answerable = discussion.category.format === 'qa'
    return (
      <div>
        <nav className="breadcrumb" aria-label="Breadcrumb">
          <ol>
            <li>
              <Link to="/w/$ws/discussions" params={{ ws }} activeOptions={{ exact: true }}>
                Discussions
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
                await navigate({ to: '/w/$ws/discussions', params: { ws } })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getDiscussionMarkdown(ws, did)}
        />

        <section id="comments" className="comments">
          <h2>
            Comments
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
            onReply={(tid, body) => mutate(() => api.createReply(ws, did, tid, body))}
            replyHandlers={{
              pending: isPending,
              onEdit: (tid, cid, body) => mutate(() => api.updateReply(ws, did, tid, cid, body)),
              onDelete: (tid, cid) => mutate(() => api.deleteReply(ws, did, tid, cid)),
              onHide: (tid, cid, reason) => mutate(() => api.hideReply(ws, did, tid, cid, reason)),
              onUnhide: (tid, cid) => mutate(() => api.unhideReply(ws, did, tid, cid)),
            }}
          />
          <CommentForm
            submitting={isPending}
            onSubmit={(body) => mutate(() => api.createThread(ws, did, body))}
          />
        </section>

        <a className="back-to-top" href="#main-content" aria-label="Back to top">
          ↑ Top
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
        } catch (err) {
          setErrors(apiFieldErrors(err))
        } finally {
          setPending(false)
          void router.invalidate()
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
