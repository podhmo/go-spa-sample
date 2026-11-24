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
import { IssueDetail } from './components/IssueDetail'
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
  component: () => (
    <>
      <nav style={{ display: 'flex', gap: '1rem' }}>
        <Link to="/issues">Issues</Link>
        <Link to="/issues/new">New issue</Link>
        <Link to="/labels">Labels</Link>
      </nav>
      <hr />
      <Outlet />
    </>
  ),
  notFoundComponent: () => (
    <div>
      <h2>404 — not found</h2>
      <p>
        The page or resource you are looking for does not exist.{' '}
        <Link to="/issues">Back to issues</Link>
      </p>
    </div>
  ),
  errorComponent: ({ error }) => (
    <div>
      <h2>Something went wrong</h2>
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

type IssueSearch = { status?: string; q?: string; label?: string; sort?: string; page?: number }

const str = (v: unknown) => (typeof v === 'string' && v !== '' ? v : undefined)
const num = (v: unknown) => {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number(v) : NaN
  return Number.isInteger(n) && n >= 1 ? n : undefined
}

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
    const totalPages = Math.max(1, Math.ceil(data.total / data.per_page))
    return (
      <div>
        <h2>Issues</h2>
        <search>
          {/* key remounts the uncontrolled inputs when the URL filters
              change (e.g. pagination links), keeping defaults in sync */}
          <form
            key={JSON.stringify(search)}
            aria-label="filter issues"
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
            <label htmlFor="filter-status" className="visually-hidden">
              Status
            </label>
            <select id="filter-status" name="status" defaultValue={search.status ?? 'open'}>
              <option value="all">all</option>
              <option value="open">open</option>
              <option value="closed">closed</option>
            </select>{' '}
            <label htmlFor="filter-label" className="visually-hidden">
              Label
            </label>
            <select id="filter-label" name="label" defaultValue={search.label ?? ''}>
              <option value="">any label</option>
              {labels.map((l) => (
                <option key={l.id} value={l.name}>
                  {l.name}
                </option>
              ))}
            </select>{' '}
            <label htmlFor="filter-sort" className="visually-hidden">
              Sort
            </label>
            <select id="filter-sort" name="sort" defaultValue={search.sort ?? 'newest'}>
              <option value="newest">newest</option>
              <option value="oldest">oldest</option>
            </select>{' '}
            <label htmlFor="filter-q" className="visually-hidden">
              Search
            </label>
            <input
              id="filter-q"
              type="search"
              name="q"
              placeholder="search"
              defaultValue={search.q}
            />{' '}
            <button type="submit">filter</button>
          </form>
        </search>
        <IssueList issues={data.issues} />
        <p>
          {data.total} issues (page {data.page} / {totalPages}){' '}
          {data.page > 1 && (
            <>
              <Link to="/issues" search={{ ...search, page: data.page - 1 }}>
                prev
              </Link>{' '}
            </>
          )}
          {data.page < totalPages && (
            <Link to="/issues" search={{ ...search, page: data.page + 1 }}>
              next
            </Link>
          )}
        </p>
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
        <h2>New issue</h2>
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
        <h2>Edit issue #{issue.id}</h2>
        <IssueForm
          submitting={isPending}
          errors={errors}
          labels={labels}
          initial={{
            title: issue.title,
            body: issue.body,
            label_ids: issue.labels.map((l) => l.id),
          }}
          submitLabel="save"
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
        <IssueDetail
          issue={issue}
          comments={comments}
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
          commentHandlers={{
            onEdit: (cid, body) => mutate(() => api.updateComment(WS, issueID, cid, body)),
            onDelete: (cid) => mutate(() => api.deleteComment(WS, issueID, cid)),
            onHide: (cid, reason) => mutate(() => api.hideComment(WS, issueID, cid, reason)),
            onUnhide: (cid) => mutate(() => api.unhideComment(WS, issueID, cid)),
          }}
        />
        <CommentForm
          submitting={isPending}
          onSubmit={(body) => mutate(() => api.createComment(WS, issueID, body))}
        />
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
