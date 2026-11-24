import { Link } from '@tanstack/react-router'
import type { Issue, IssueListFilter, Label } from '../api'
import { fmtTime, labelStyle } from '../format'

export function StatusBadge({ status }: { status: Issue['status'] }) {
  return <span className={`badge badge-${status}`}>{status === 'open' ? 'Open' : 'Closed'}</span>
}

export function LabelBadge({ label }: { label: Label }) {
  return (
    <span className="label-badge" style={labelStyle(label)}>
      {label.name}
    </span>
  )
}

export function IssueList({
  issues,
  filtered,
  search,
}: {
  issues: Issue[]
  // Whether a filter (other than the status tabs) is active — picks the
  // MPA's filtered empty state ("No issues match the current filters.")
  // over the blank-slate one.
  filtered?: boolean | undefined
  // Current URL filters — label badges re-filter the list through them,
  // like the MPA's labelURL. Optional for standalone component tests.
  search?: IssueListFilter | undefined
}) {
  if (issues.length === 0) {
    if (filtered) {
      return (
        <div className="empty-state">
          <p className="empty-lead">No issues match the current filters.</p>
          <p>
            Try a different keyword or <Link to="/issues">clear the filters</Link>.
          </p>
        </div>
      )
    }
    return (
      <div className="empty-state">
        <p className="empty-lead">No issues here yet.</p>
        <p>
          <Link className="btn btn-primary" to="/issues/new">
            Create the first issue
          </Link>
        </p>
      </div>
    )
  }
  return (
    <ul className="issue-list" role="list">
      {issues.map((issue) => (
        <li className="issue-row" key={issue.id}>
          <div className="issue-row-main">
            <Link
              className="issue-title"
              to="/issues/$issueId"
              params={{ issueId: String(issue.id) }}
            >
              {issue.title}
            </Link>
            <span className="issue-num">#{issue.id}</span>
          </div>
          <div className="issue-row-meta">
            <StatusBadge status={issue.status} />
            {issue.labels.map((l) => (
              <Link
                className="label-badge"
                style={labelStyle(l)}
                key={l.id}
                to="/issues"
                search={{ ...search, label: l.name, page: undefined }}
              >
                {l.name}
              </Link>
            ))}
            <time className="issue-time" dateTime={issue.created_at}>
              opened {fmtTime(issue.created_at)}
            </time>
          </div>
        </li>
      ))}
    </ul>
  )
}
