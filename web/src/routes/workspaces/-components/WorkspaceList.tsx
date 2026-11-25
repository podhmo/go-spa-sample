import { Link } from '@tanstack/react-router'
import type { Workspace } from '../../../api'
import { fmt, useMessages } from '../../../i18n'
import { ConfirmDialog } from '../../../components/ConfirmDialog'
import styles from './WorkspaceList.module.css'

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
  const m = useMessages()
  if (workspaces.length === 0) {
    return <p className="empty-state">{m.workspaces.empty}</p>
  }
  const isLast = workspaces.length <= 1
  return (
    <ul className={styles.workspaceList}>
      {workspaces.map((w) => (
        <li className={styles.workspaceRow} key={w.id}>
          <div className={styles.workspaceMain}>
            <Link className={styles.workspaceSlug} to="/w/$ws/issues" params={{ ws: w.slug }}>
              w/{w.slug}
            </Link>
            {w.name !== w.slug && <span className={styles.workspaceName}>{w.name}</span>}
            {w.description !== '' && <p className={styles.workspaceDesc}>{w.description}</p>}
          </div>
          <div className={styles.workspaceMeta}>
            <span>{fmt(m.workspaces.issuesCount, { n: w.issue_count })}</span>
            <span>{fmt(m.workspaces.discussionsCount, { n: w.discussion_count })}</span>
            {isLast ? (
              <span className="muted">{m.workspaces.lastUndeletable}</span>
            ) : (
              <ConfirmDialog
                label={m.workspaces.delete}
                title={fmt(m.workspaces.deleteTitle, { slug: w.slug })}
                description={fmt(m.workspaces.deleteBody, {
                  slug: w.slug,
                  issues: w.issue_count,
                  discussions: w.discussion_count,
                })}
                confirmLabel={m.workspaces.deleteConfirm}
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
