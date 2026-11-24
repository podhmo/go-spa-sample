import { Link } from '@tanstack/react-router'
import type { Comment, Issue } from '../api'
import { ConfirmDialog } from './ConfirmDialog'
import { LabelBadge, StatusBadge } from './IssueList'

export function CommentList({ comments }: { comments: Comment[] }) {
  if (comments.length === 0) {
    return <p>No comments yet.</p>
  }
  return (
    <ul>
      {comments.map((c) => (
        <li key={c.id} data-hidden={c.hidden || undefined}>
          {c.hidden ? (
            <em>This comment was hidden{c.hidden_reason ? ` (${c.hidden_reason})` : ''}.</em>
          ) : (
            <p style={{ whiteSpace: 'pre-wrap' }}>{c.body}</p>
          )}
          <small>
            {c.author ? c.author.name : 'anonymous'} — {c.created_at}
          </small>
        </li>
      ))}
    </ul>
  )
}

export function IssueDetail({
  issue,
  comments,
  onSetStatus,
  onDelete,
  pending,
}: {
  issue: Issue
  comments: Comment[]
  onSetStatus?: (status: 'open' | 'closed') => void
  onDelete?: () => void
  pending?: boolean
}) {
  const next = issue.status === 'open' ? 'closed' : 'open'
  return (
    <article>
      <header>
        <h2>
          #{issue.id} {issue.title} <StatusBadge status={issue.status} />
        </h2>
        <p>
          {issue.labels.map((l) => (
            <LabelBadge key={l.id} label={l} />
          ))}{' '}
          {issue.author ? `opened by ${issue.author.name}` : ''} — {issue.created_at}
        </p>
        {onSetStatus && (
          <button onClick={() => onSetStatus(next)} disabled={pending}>
            {pending ? '…' : `mark ${next}`}
          </button>
        )}{' '}
        <Link to="/issues/$issueId/edit" params={{ issueId: String(issue.id) }}>
          edit
        </Link>{' '}
        {onDelete && (
          <ConfirmDialog
            label="delete"
            title={`Delete issue #${issue.id}?`}
            confirmLabel="delete"
            pending={pending}
            onConfirm={onDelete}
          />
        )}
      </header>
      <section>
        <p style={{ whiteSpace: 'pre-wrap' }}>{issue.body}</p>
      </section>
      <section>
        <h3>Comments ({comments.length})</h3>
        <CommentList comments={comments} />
      </section>
    </article>
  )
}
