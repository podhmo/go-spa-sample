import { createFileRoute, Link, useNavigate, useRouter } from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { api } from '../../api'
import { apiFieldErrors } from '../-lib/mutation'
import { WorkspaceForm } from './-components/WorkspaceForm'
import { useMessages } from '../../i18n'

export const Route = createFileRoute('/workspaces/new')({
  component: function NewWorkspacePage() {
    const navigate = useNavigate()
    const router = useRouter()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.workspaces.new}</h1>
        </div>
        <WorkspaceForm
          submitting={isPending}
          errors={errors}
          cancelLink={
            <Link className="btn btn-quiet" to="/workspaces">
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { workspace } = await api.createWorkspace(input)
                void router.invalidate()
                await navigate({ to: '/w/$ws/issues', params: { ws: workspace.slug } })
              } catch (err) {
                setErrors(apiFieldErrors(err, 'slug'))
              }
            })
          }}
        />
      </div>
    )
  },
})
