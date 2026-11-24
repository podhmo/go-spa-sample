import { Link } from '@tanstack/react-router'
import type { Issue, Label } from '../api'

export function StatusBadge({ status }: { status: Issue['status'] }) {
  return <span data-status={status}>{status}</span>
}

export function LabelBadge({ label }: { label: Label }) {
  return (
    <span
      style={{
        backgroundColor: label.color,
        color: 'black',
        borderRadius: '0.25rem',
        padding: '0 0.4em',
        fontSize: '0.85em',
      }}
    >
      {label.name}
    </span>
  )
}

export function IssueList({ issues }: { issues: Issue[] }) {
  if (issues.length === 0) {
    return <p>No issues.</p>
  }
  return (
    <ul>
      {issues.map((issue) => (
        <li key={issue.id}>
          <Link to="/issues/$issueId" params={{ issueId: String(issue.id) }}>
            #{issue.id} {issue.title}
          </Link>{' '}
          <StatusBadge status={issue.status} />{' '}
          {issue.labels.map((l) => (
            <LabelBadge key={l.id} label={l} />
          ))}{' '}
          {issue.author ? `by ${issue.author.name}` : ''}
        </li>
      ))}
    </ul>
  )
}
