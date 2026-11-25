import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { api } from '../../../../api'
import { scoped } from '../../../-lib/scoped'
import { issueSearch, str } from '../../../-lib/search'
import { IssueList } from './-components/IssueList'
import { fmt, useMessages } from '../../../../i18n'

const STATUS_TABS = ['open', 'closed', 'all'] as const

export const Route = createFileRoute('/w/$ws/issues/')({
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
    const { list: data, labels } = Route.useLoaderData()
    const { ws } = Route.useParams()
    const search = Route.useSearch()
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
