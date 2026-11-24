import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  isNotFound,
  notFound,
  redirect,
  useNavigate,
  useRouter,
} from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { RequestError, WS, api } from './api'
import { CommentForm, IssueForm } from './components/forms'
import { CommentList, IssueDetail } from './components/IssueDetail'
import { IssueList } from './components/IssueList'
import { LabelsAdmin } from './components/LabelsAdmin'

// Rethrows a 404 RequestError as the router's notFound() so a missing
// resource shows the notFoundComponent instead of a generic error.
const notFoundOn404 = async <T,>(p: Promise<T>): Promise<T> => {
  try {
    return await p
  } catch (err) {
    if (err instanceof RequestError && err.status === 404) throw notFound()
    throw err
  }
}

const rootRoute = createRootRoute({
  // Same page chrome as the MPA's layout.html: skip link, sticky header
  // (site title + nav + primary action), main landmark, footer.
  component: () => (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <header className="site-header">
        <div className="header-bar">
          <div className="container header-inner">
            <Link className="site-title" to="/issues">
              zatudan
            </Link>
            <nav className="site-nav" aria-label="Main">
              <Link to="/issues">Issues</Link>
              <Link to="/labels">Labels</Link>
              <Link className="btn" to="/issues/new">
                New issue
              </Link>
            </nav>
          </div>
        </div>
      </header>
      <main className="container main-content" id="main-content" tabIndex={-1}>
        <Outlet />
      </main>
      <footer className="site-footer">
        <div className="container">
          <p>
            A single-page app port of zatudan — Go API + React, TanStack Router, minimal JavaScript
            where it earns its place.
          </p>
        </div>
      </footer>
    </>
  ),
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

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/issues' })
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

const STATUS_TABS = ['open', 'closed', 'all'] as const

const issuesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues',
  validateSearch: (search: Record<string, unknown>): IssueSearch => ({
    status: str(search.status),
    q: str(search.q),
    label: str(search.label),
    sort: str(search.sort),
    page: num(search.page),
  }),
  loaderDeps: ({ search }) => search,
  loader: async ({ deps }) => {
    const [list, labels] = await Promise.all([
      api.listIssues(WS, deps),
      api.listLabels(WS, { kind: 'issue' }),
    ])
    return { list, labels: labels.labels }
  },
  component: function IssuesPage() {
    const { list: data, labels } = issuesRoute.useLoaderData()
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
              to="/issues"
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
                to: '/issues',
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
                <Link className="btn btn-quiet" to="/issues">
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
        <IssueList issues={data.issues} filtered={hasActiveFilter} search={search} />

        {totalPages > 1 && (
          <nav className="pagination" aria-label="Pagination">
            {data.page > 1 ? (
              <Link className="btn" to="/issues" search={{ ...search, page: data.page - 1 }}>
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
              <Link className="btn" to="/issues" search={{ ...search, page: data.page + 1 }}>
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
// 422 ({"error":{"fields":{...}}}), falling back to a generic title error.
const apiFieldErrors = (err: unknown): Record<string, string> =>
  err instanceof RequestError && err.apiError?.fields
    ? err.apiError.fields
    : { title: err instanceof Error ? err.message : String(err) }

const newIssueRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues/new',
  loader: () => api.listLabels(WS, { kind: 'issue' }),
  component: function NewIssuePage() {
    const { labels } = newIssueRoute.useLoaderData()
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
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { issue } = await api.createIssue(WS, input)
                await navigate({
                  to: '/issues/$issueId',
                  params: { issueId: String(issue.id) },
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
  getParentRoute: () => rootRoute,
  path: '/issues/$issueId/edit',
  loader: async ({ params }) => {
    const [detail, labels] = await notFoundOn404(
      Promise.all([
        api.getIssue(WS, Number(params.issueId)),
        api.listLabels(WS, { kind: 'issue' }),
      ]),
    )
    return { ...detail, labels: labels.labels }
  },
  component: function EditIssuePage() {
    const { issue, labels } = editIssueRoute.useLoaderData()
    const { issueId } = editIssueRoute.useParams()
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
            <Link className="btn btn-quiet" to="/issues/$issueId" params={{ issueId }}>
              Cancel
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                await api.updateIssue(WS, Number(issueId), input)
                await navigate({
                  to: '/issues/$issueId',
                  params: { issueId },
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
  getParentRoute: () => rootRoute,
  path: '/issues/$issueId',
  loader: ({ params }) => notFoundOn404(api.getIssue(WS, Number(params.issueId))),
  component: function IssuePage() {
    const { issue, comments } = issueRoute.useLoaderData()
    const { issueId } = issueRoute.useParams()
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
              <Link to="/issues" activeOptions={{ exact: true }}>
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
          pending={isPending}
          onSetStatus={(status) => mutate(() => api.setIssueStatus(WS, issueID, status))}
          onDelete={() => {
            setPending(true)
            void (async () => {
              try {
                await api.deleteIssue(WS, issueID)
                await navigate({ to: '/issues' })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getIssueMarkdown(WS, issueID)}
        />

        <section id="comments" className="comments">
          <h2>
            Comments
            {comments.length > 0 && <span className="comment-count"> ({comments.length})</span>}
          </h2>
          <CommentList
            comments={comments}
            pending={isPending}
            onEdit={(cid, body) => mutate(() => api.updateComment(WS, issueID, cid, body))}
            onDelete={(cid) => mutate(() => api.deleteComment(WS, issueID, cid))}
            onHide={(cid, reason) => mutate(() => api.hideComment(WS, issueID, cid, reason))}
            onUnhide={(cid) => mutate(() => api.unhideComment(WS, issueID, cid))}
          />
          <CommentForm
            submitting={isPending}
            onSubmit={(body) => mutate(() => api.createComment(WS, issueID, body))}
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
  getParentRoute: () => rootRoute,
  path: '/labels',
  loader: async () => {
    const [active, archived] = await Promise.all([
      api.listLabels(WS),
      api.listLabels(WS, { archived: true }),
    ])
    return { active: active.labels, archived: archived.labels }
  },
  component: function LabelsPage() {
    const { active, archived } = labelsRoute.useLoaderData()
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
        onCreate={(input) => mutate(() => api.createLabel(WS, input))}
        onUpdate={(id, patch) => mutate(() => api.updateLabel(WS, id, patch))}
        onArchive={(id) => mutate(() => api.archiveLabel(WS, id))}
        onRestore={(l) =>
          mutate(() =>
            api.createLabel(WS, {
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
  issuesRoute,
  newIssueRoute,
  issueRoute,
  editIssueRoute,
  labelsRoute,
])
