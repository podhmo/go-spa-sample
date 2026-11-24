import { Link } from '@tanstack/react-router'
import type { Discussion } from '../api'
import { fmtTime, labelStyle } from '../format'

export function AnsweredBadge() {
  return <span className="badge badge-answered">Answered</span>
}

export function CategoryBadge({ name }: { name: string }) {
  return <span className="badge badge-category">{name}</span>
}

export function DiscussionList({
  discussions,
  ws,
  filtered,
}: {
  discussions: Discussion[]
  // The workspace this list is scoped to — its links stay under /w/{ws}.
  ws: string
  // Whether a non-default filter is active — picks the filtered empty
  // state over the blank-slate one.
  filtered?: boolean | undefined
}) {
  if (discussions.length === 0) {
    if (filtered) {
      return (
        <div className="empty-state">
          <p className="empty-lead">No discussions match the current filters.</p>
          <p>
            Try a different keyword or{' '}
            <Link to="/w/$ws/discussions" params={{ ws }}>
              clear the filters
            </Link>
            .
          </p>
        </div>
      )
    }
    return (
      <div className="empty-state">
        <p className="empty-lead">No discussions here yet.</p>
        <p>
          <Link className="btn btn-primary" to="/w/$ws/discussions/new" params={{ ws }}>
            Start the first discussion
          </Link>
        </p>
      </div>
    )
  }
  return (
    <ul className="issue-list" role="list">
      {discussions.map((d) => (
        <li className="issue-row" key={d.id}>
          <div className="issue-row-main">
            <Link
              className="issue-title"
              to="/w/$ws/discussions/$discussionId"
              params={{ ws, discussionId: String(d.id) }}
            >
              {d.title}
            </Link>
            <span className="issue-num">#{d.id}</span>
          </div>
          <div className="issue-row-meta">
            <CategoryBadge name={d.category.name} />
            {d.answered && <AnsweredBadge />}
            {d.labels.map((l) => (
              <span className="label-badge" style={labelStyle(l)} key={l.id}>
                {l.name}
              </span>
            ))}
            <time className="issue-time" dateTime={d.created_at}>
              opened {fmtTime(d.created_at)}
            </time>
          </div>
        </li>
      ))}
    </ul>
  )
}
