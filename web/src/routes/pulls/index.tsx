import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { api } from '../../api'
import { pullSearch, str } from '../-lib/search'
import { PullList } from './-components/PullList'
import { pullOrNotFound } from './-lib/pull-or-not-found'
import { fmt, useMessages } from '../../i18n'

export const Route = createFileRoute('/pulls/')({
  validateSearch: pullSearch,
  loaderDeps: ({ search }) => search,
  loader: ({ deps }) => pullOrNotFound(api.listPulls(deps)),
  component: function PullsPage() {
    const data = Route.useLoaderData()
    const search = Route.useSearch()
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
