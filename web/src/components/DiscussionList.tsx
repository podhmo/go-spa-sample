import { Link } from '@tanstack/react-router'
import type { Discussion } from '../api'
import { labelStyle } from '../format'
import { fmt, useFmtTime, useMessages } from '../i18n'

export function AnsweredBadge() {
  const m = useMessages()
  return <span className="badge badge-answered">{m.discussions.answered}</span>
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
  const m = useMessages()
  const fmtTime = useFmtTime()
  if (discussions.length === 0) {
    if (filtered) {
      return (
        <div className="empty-state">
          <p className="empty-lead">{m.discussions.filteredEmpty}</p>
          <p>
            {m.issues.tryOther}{' '}
            <Link to="/w/$ws/discussions" params={{ ws }}>
              {m.issues.clearFilters}
            </Link>
            .
          </p>
        </div>
      )
    }
    return (
      <div className="empty-state">
        <p className="empty-lead">{m.discussions.blankEmpty}</p>
        <p>
          <Link className="btn btn-primary" to="/w/$ws/discussions/new" params={{ ws }}>
            {m.discussions.createFirst}
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
              {fmt(m.issues.openedAt, { time: fmtTime(d.created_at) })}
            </time>
          </div>
        </li>
      ))}
    </ul>
  )
}
