import { Link } from '@tanstack/react-router'
import type { Issue, IssueListFilter } from '../../../../../api'
import { labelStyle } from '../../../../../format'
import { fmt, useFmtTime, useMessages } from '../../../../../i18n'
import { cx } from '../../../../../styles/cx'

export function StatusBadge({ status }: { status: Issue['status'] }) {
  const m = useMessages()
  return (
    <span className={cx(`badge badge-${status}`)}>
      {status === 'open' ? m.issues.badgeOpen : m.issues.badgeClosed}
    </span>
  )
}

export function IssueList({
  issues,
  ws,
  filtered,
  search,
}: {
  issues: Issue[]
  // The workspace this list is scoped to — its links stay under /w/{ws}.
  ws: string
  // Whether a filter (other than the status tabs) is active — picks the
  // MPA's filtered empty state ("No issues match the current filters.")
  // over the blank-slate one.
  filtered?: boolean | undefined
  // Current URL filters — label badges re-filter the list through them,
  // like the MPA's labelURL. Optional for standalone component tests.
  search?: IssueListFilter | undefined
}) {
  const m = useMessages()
  const fmtTime = useFmtTime()
  if (issues.length === 0) {
    if (filtered) {
      return (
        <div className={cx('empty-state')}>
          <p className={cx('empty-lead')}>{m.issues.filteredEmpty}</p>
          <p>
            {m.issues.tryOther}{' '}
            <Link to="/w/$ws/issues" params={{ ws }}>
              {m.issues.clearFilters}
            </Link>
            .
          </p>
        </div>
      )
    }
    return (
      <div className={cx('empty-state')}>
        <p className={cx('empty-lead')}>{m.issues.blankEmpty}</p>
        <p>
          <Link className={cx('btn btn-primary')} to="/w/$ws/issues/new" params={{ ws }}>
            {m.issues.createFirst}
          </Link>
        </p>
      </div>
    )
  }
  return (
    <ul className={cx('issue-list')} role="list">
      {issues.map((issue) => (
        <li className={cx('issue-row')} key={issue.id}>
          <div className={cx('issue-row-main')}>
            <Link
              className={cx('issue-title')}
              to="/w/$ws/issues/$issueId"
              params={{ ws, issueId: String(issue.id) }}
            >
              {issue.title}
            </Link>
            <span className={cx('issue-num')}>#{issue.id}</span>
          </div>
          <div className={cx('issue-row-meta')}>
            <StatusBadge status={issue.status} />
            {issue.labels.map((l) => (
              <Link
                className={cx('label-badge')}
                style={labelStyle(l)}
                key={l.id}
                to="/w/$ws/issues"
                params={{ ws }}
                search={{ ...search, label: l.name, page: undefined }}
              >
                {l.name}
              </Link>
            ))}
            <time className={cx('issue-time')} dateTime={issue.created_at}>
              {fmt(m.issues.openedAt, { time: fmtTime(issue.created_at) })}
            </time>
          </div>
        </li>
      ))}
    </ul>
  )
}
