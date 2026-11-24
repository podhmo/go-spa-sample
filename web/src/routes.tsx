import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  redirect,
  useNavigate,
  useRouter,
} from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { WS, api } from './api'
import { CommentForm, IssueForm } from './components/forms'
import { IssueDetail } from './components/IssueDetail'
import { IssueList } from './components/IssueList'

const rootRoute = createRootRoute({
  component: () => (
    <>
      <nav style={{ display: 'flex', gap: '1rem' }}>
        <Link to="/issues">Issues</Link>
        <Link to="/issues/new">New issue</Link>
      </nav>
      <hr />
      <Outlet />
    </>
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

const newIssueRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues/new',
  component: function NewIssuePage() {
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
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { issue } = await api.createIssue(WS, input)
                await navigate({
                  to: '/issues/$issueId',
                  params: { issueId: String(issue.id) },
                })
              } catch (err) {
                setErrors(
                  err instanceof Error && 'apiError' in err
                    ? ((err as { apiError: { fields?: Record<string, string> } }).apiError
                        ?.fields ?? {
                        title: err.message,
                      })
                    : { title: String(err) },
                )
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
  loader: ({ params }) => api.getIssue(WS, Number(params.issueId)),
  component: function IssuePage() {
    const { issue, comments } = issueRoute.useLoaderData()
    const { issueId } = issueRoute.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    return (
      <div>
        <IssueDetail
          issue={issue}
          comments={comments}
          pending={isPending}
          onSetStatus={(status) => {
            startTransition(async () => {
              await api.setIssueStatus(WS, Number(issueId), status)
              await router.invalidate()
            })
          }}
          onDelete={() => {
            startTransition(async () => {
              await api.deleteIssue(WS, Number(issueId))
              await navigate({ to: '/issues' })
            })
          }}
        />
        <CommentForm
          submitting={isPending}
          onSubmit={(body) => {
            startTransition(async () => {
              await api.createComment(WS, Number(issueId), body)
              await router.invalidate()
            })
          }}
        />
      </div>
    )
  },
})

export const routeTree = rootRoute.addChildren([indexRoute, issuesRoute, newIssueRoute, issueRoute])
