import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { api } from '../../../../api'
import { scoped } from '../../../-lib/scoped'
import { discussionSearch, str } from '../../../-lib/search'
import { DiscussionList } from './-components/DiscussionList'
import { fmt, useMessages } from '../../../../i18n'
import { cx } from '../../../../styles/cx'

export const Route = createFileRoute('/w/$ws/discussions/')({
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
    const { list: data, categories } = Route.useLoaderData()
    const { ws } = Route.useParams()
    const search = Route.useSearch()
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
        <div className={cx('page-head')}>
          <h1>{m.discussions.title}</h1>
          <Link className={cx('btn')} to="/w/$ws/discussions/new" params={{ ws }}>
            {m.discussions.new}
          </Link>
        </div>

        <search>
          {/* key remounts the uncontrolled inputs when the URL filters change */}
          <form
            key={JSON.stringify(search)}
            className={cx('filter-bar')}
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
            <div className={cx('filter-field')}>
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
            <div className={cx('filter-field')}>
              <label htmlFor="filter-answered">{m.filter.answered}</label>
              <select id="filter-answered" name="answered" defaultValue={search.answered ?? ''}>
                <option value="">{m.filter.all}</option>
                <option value="yes">{m.filter.answered}</option>
                <option value="no">{m.filter.unanswered}</option>
              </select>
            </div>
            <div className={cx('filter-field')}>
              <label htmlFor="filter-dq">{m.filter.search}</label>
              <input
                type="search"
                id="filter-dq"
                name="q"
                placeholder={m.filter.searchPh}
                defaultValue={search.q}
              />
            </div>
            <div className={cx('filter-field')}>
              <label htmlFor="filter-dsort">{m.filter.sort}</label>
              <select id="filter-dsort" name="sort" defaultValue={search.sort ?? 'newest'}>
                <option value="newest">{m.filter.newest}</option>
                <option value="oldest">{m.filter.oldest}</option>
              </select>
            </div>
            <div className={cx('filter-actions')}>
              <button type="submit" className={cx('btn')}>
                {m.filter.apply}
              </button>
              {hasActiveFilter && (
                <Link className={cx('btn btn-quiet')} to="/w/$ws/discussions" params={{ ws }}>
                  {m.filter.clear}
                </Link>
              )}
            </div>
          </form>
        </search>

        {data.discussions.length > 0 && (
          <p className={cx('result-count')}>
            {fmt(m.pager.showingDiscussions, {
              start: rangeStart,
              end: rangeEnd,
              total: data.total,
            })}
          </p>
        )}
        <DiscussionList discussions={data.discussions} ws={ws} filtered={hasActiveFilter} />

        {totalPages > 1 && (
          <nav className={cx('pagination')} aria-label={m.pager.aria}>
            {data.page > 1 ? (
              <Link
                className={cx('btn')}
                to="/w/$ws/discussions"
                params={{ ws }}
                search={{ ...search, page: data.page - 1 }}
              >
                {m.pager.prev}
              </Link>
            ) : (
              <span className={cx('btn btn-disabled')} aria-disabled="true">
                {m.pager.prev}
              </span>
            )}
            <span className={cx('page-info')}>
              {fmt(m.pager.info, { page: data.page, total: totalPages })}
            </span>
            {data.page < totalPages ? (
              <Link
                className={cx('btn')}
                to="/w/$ws/discussions"
                params={{ ws }}
                search={{ ...search, page: data.page + 1 }}
              >
                {m.pager.next}
              </Link>
            ) : (
              <span className={cx('btn btn-disabled')} aria-disabled="true">
                {m.pager.next}
              </span>
            )}
          </nav>
        )}
      </div>
    )
  },
})
