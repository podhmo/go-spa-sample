import { createFileRoute, Link, useRouter } from '@tanstack/react-router'
import { useReducer } from 'react'
import { api } from '../../api'
import { WorkspaceList } from './-components/WorkspaceList'
import { useMessages } from '../../i18n'

export const Route = createFileRoute('/workspaces/')({
  loader: () => api.listWorkspaces(),
  component: function WorkspacesPage() {
    const { workspaces } = Route.useLoaderData()
    const router = useRouter()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [error, setError] = useReducer((_: string | null, v: string | null) => v, null)
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      void (async () => {
        try {
          await fn()
          setError(null)
          void router.invalidate()
        } catch (err) {
          setError(err instanceof Error ? err.message : String(err))
        } finally {
          setPending(false)
        }
      })()
    }
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.workspaces.title}</h1>
          <Link className="btn" to="/workspaces/new">
            {m.workspaces.new}
          </Link>
        </div>
        {/* em role=alert is the always-visible server-error idiom;
            .error-msg alone stays hidden (field-sibling reveal only) */}
        {error && <em role="alert">{error}</em>}
        <WorkspaceList
          workspaces={workspaces}
          pending={isPending}
          onDelete={(slug) => mutate(() => api.deleteWorkspace(slug))}
        />
      </div>
    )
  },
})
