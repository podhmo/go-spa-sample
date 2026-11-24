import { Link } from '@tanstack/react-router'
import type { Workspace } from '../api'
import { ConfirmDialog } from './ConfirmDialog'

// The MPA's /workspaces index: one row per workspace — slug link, name
// (when it differs from the slug), description, counts, delete action.
export function WorkspaceList({
  workspaces,
  pending,
  onDelete,
}: {
  workspaces: Workspace[]
  pending?: boolean | undefined
  onDelete: (slug: string) => void
}) {
  if (workspaces.length === 0) {
    return <p className="empty-state">No workspaces yet.</p>
  }
  const isLast = workspaces.length <= 1
  return (
    <ul className="workspace-list">
      {workspaces.map((w) => (
        <li className="workspace-row" key={w.id}>
          <div className="workspace-main">
            <Link className="workspace-slug" to="/w/$ws/issues" params={{ ws: w.slug }}>
              w/{w.slug}
            </Link>
            {w.name !== w.slug && <span className="workspace-name">{w.name}</span>}
            {w.description !== '' && <p className="workspace-desc">{w.description}</p>}
          </div>
          <div className="workspace-meta">
            <span>{w.issue_count} issues</span>
            <span>{w.discussion_count} discussions</span>
            {isLast ? (
              <span className="muted">The last remaining workspace cannot be deleted.</span>
            ) : (
              <ConfirmDialog
                label="Delete"
                title={`Delete workspace ${w.slug}?`}
                description={`This deletes the workspace w/${w.slug} and everything in it — ${w.issue_count} issues and ${w.discussion_count} discussions. This cannot be undone.`}
                confirmLabel="Delete workspace"
                pending={pending}
                onConfirm={() => onDelete(w.slug)}
              />
            )}
          </div>
        </li>
      ))}
    </ul>
  )
}
