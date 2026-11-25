import { Link } from '@tanstack/react-router'
import type { PullRequest } from '../../../api'
import { fmt, useFmtTime, useMessages } from '../../../i18n'
import styles from './pulls.module.css'
import { cx } from '../../../styles/cx'

const statusClasses = {
  open: styles.badgePrOpen,
  closed: styles.badgePrClosed,
  merged: styles.badgePrMerged,
} as const

export function PullStatusBadge({ status }: { status: PullRequest['status'] }) {
  const m = useMessages()
  const text =
    status === 'open' ? m.filter.open : status === 'closed' ? m.filter.closed : m.filter.merged
  return <span className={`badge ${statusClasses[status]}`}>{text}</span>
}

const shortRef = (ref: string) => ref.replace(/^refs\/(heads|remotes|tags)\//, '')

export function PullList({
  pulls,
  filtered,
}: {
  pulls: PullRequest[]
  filtered?: boolean | undefined
}) {
  const m = useMessages()
  const fmtTime = useFmtTime()
  if (pulls.length === 0) {
    if (filtered) {
      return (
        <div className="empty-state">
          <p className="empty-lead">{m.pulls.filteredEmpty}</p>
          <p>
            {m.issues.tryOther} <Link to="/pulls">{m.issues.clearFilters}</Link>.
          </p>
        </div>
      )
    }
    return (
      <div className="empty-state">
        <p className="empty-lead">{m.pulls.blankEmpty}</p>
        <p>
          <Link className={cx('btn btn-primary')} to="/pulls/new">
            {m.pulls.openFirst}
          </Link>
        </p>
      </div>
    )
  }
  return (
    <ul className="issue-list" role="list">
      {pulls.map((p) => (
        <li className="issue-row" key={p.id}>
          <div className="issue-row-main">
            <Link
              preload={false}
              className="issue-title"
              to="/pulls/$pullId"
              params={{ pullId: String(p.id) }}
            >
              {p.title}
            </Link>
            <span className="issue-num">#{p.id}</span>
            <PullStatusBadge status={p.status} />
          </div>
          <p className="issue-row-meta">
            <code className={styles.prRef}>{shortRef(p.head)}</code> →{' '}
            <code className={styles.prRef}>{shortRef(p.base)}</code> ·{' '}
            {fmt(m.pulls.openedBy, { name: p.author?.name ?? m.comments.anonymous })}{' '}
            <time dateTime={p.created_at}>{fmtTime(p.created_at)}</time>
          </p>
        </li>
      ))}
    </ul>
  )
}
