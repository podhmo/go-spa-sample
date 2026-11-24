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

type IssueSearch = { status?: string; q?: string }

const issuesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/issues',
  validateSearch: (search: Record<string, unknown>): IssueSearch => ({
    status: typeof search.status === 'string' ? search.status : undefined,
    q: typeof search.q === 'string' ? search.q : undefined,
  }),
  loaderDeps: ({ search }) => search,
  loader: ({ deps }) => api.listIssues(WS, deps),
  component: function IssuesPage() {
    const data = issuesRoute.useLoaderData()
    const search = issuesRoute.useSearch()
    const navigate = useNavigate()
    return (
      <div>
        <h2>Issues</h2>
        <search>
          <form
            aria-label="filter issues"
            onSubmit={(e) => {
              e.preventDefault()
              const fd = new FormData(e.currentTarget)
              void navigate({
                to: '/issues',
                search: {
                  status: String(fd.get('status') ?? ''),
                  q: String(fd.get('q') ?? ''),
                },
              })
            }}
          >
            <label htmlFor="filter-status" className="visually-hidden">
              Status
            </label>
            <select id="filter-status" name="status" defaultValue={search.status}>
              <option value="">all</option>
              <option value="open">open</option>
              <option value="closed">closed</option>
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
          {data.total} issues (page {data.page})
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
                    ? ((err as { apiError: { fields?: Record<string, string> } }).apiError?.fields ?? {
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

export const routeTree = rootRoute.addChildren([
  indexRoute,
  issuesRoute,
  newIssueRoute,
  issueRoute,
])
