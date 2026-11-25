import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { useReducer } from 'react'
import { api } from '../../../../api'
import { apiFieldErrors } from '../../../-lib/mutation'
import { scoped } from '../../../-lib/scoped'
import { LabelsAdmin } from './-components/LabelsAdmin'

export const Route = createFileRoute('/w/$ws/labels/')({
  loader: async ({ params }) => {
    const [active, archived] = await scoped(
      Promise.all([api.listLabels(params.ws), api.listLabels(params.ws, { archived: true })]),
      (real) => {
        throw redirect({ to: '/w/$ws/labels', params: { ws: real } })
      },
    )
    return { active: active.labels, archived: archived.labels }
  },
  component: function LabelsPage() {
    const { active, archived } = Route.useLoaderData()
    const { ws } = Route.useParams()
    const router = useRouter()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      void (async () => {
        try {
          await fn()
          setErrors({})
          void router.invalidate()
        } catch (err) {
          setErrors(apiFieldErrors(err))
        } finally {
          setPending(false)
        }
      })()
    }
    return (
      <LabelsAdmin
        active={active}
        archived={archived}
        pending={isPending}
        errors={errors}
        onCreate={(input) => mutate(() => api.createLabel(ws, input))}
        onUpdate={(id, patch) => mutate(() => api.updateLabel(ws, id, patch))}
        onArchive={(id) => mutate(() => api.archiveLabel(ws, id))}
        onRestore={(l) =>
          mutate(() =>
            api.createLabel(ws, {
              name: l.name,
              color: l.color,
              kind: l.kind,
              shared: l.workspace_id == null,
            }),
          )
        }
      />
    )
  },
})
