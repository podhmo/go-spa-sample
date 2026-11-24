import { Link } from '@tanstack/react-router'
import type { PullRequest } from '../api'
import { fmtTime } from '../format'

export function PullStatusBadge({ status }: { status: PullRequest['status'] }) {
  return <span className={`badge badge-pr-${status}`}>{status}</span>
}

const shortRef = (ref: string) => ref.replace(/^refs\/(heads|remotes|tags)\//, '')

export function PullList({
  pulls,
  filtered,
}: {
  pulls: PullRequest[]
  filtered?: boolean | undefined
}) {
  if (pulls.length === 0) {
    if (filtered) {
      return (
        <div className="empty-state">
          <p className="empty-lead">No pull requests match the current filters.</p>
          <p>
            Try a different keyword or <Link to="/pulls">clear the filters</Link>.
          </p>
        </div>
      )
    }
    return (
      <div className="empty-state">
        <p className="empty-lead">No pull requests here yet.</p>
        <p>
          <Link className="btn btn-primary" to="/pulls/new">
            Open the first pull request
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
            <Link className="issue-title" to="/pulls/$pullId" params={{ pullId: String(p.id) }}>
              {p.title}
            </Link>
            <span className="issue-num">#{p.id}</span>
            <PullStatusBadge status={p.status} />
          </div>
          <p className="issue-row-meta">
            <code className="pr-ref">{shortRef(p.head)}</code> →{' '}
            <code className="pr-ref">{shortRef(p.base)}</code> · opened by{' '}
            {p.author?.name ?? 'anonymous'}{' '}
            <time dateTime={p.created_at}>{fmtTime(p.created_at)}</time>
          </p>
        </li>
      ))}
    </ul>
  )
}
