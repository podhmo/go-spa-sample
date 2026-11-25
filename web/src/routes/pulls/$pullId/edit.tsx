import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { api } from '../../../api'
import { PullForm } from '../-components/PullForm'
import { getPullOrNotFound } from '../-lib/pull-or-not-found'
import { apiFieldErrors } from '../../-lib/mutation'
import { fmt, useMessages } from '../../../i18n'
import { cx } from '../../../styles/cx'

export const Route = createFileRoute('/pulls/$pullId/edit')({
  loader: ({ params }) => getPullOrNotFound(Number(params.pullId)),
  component: function EditPullPage() {
    const { pull_request } = Route.useLoaderData()
    const { pullId } = Route.useParams()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className={cx('page-head')}>
          <h1>{fmt(m.pulls.editTitle, { id: pull_request.id })}</h1>
        </div>
        <PullForm
          submitting={isPending}
          errors={errors}
          initial={{ title: pull_request.title, body: pull_request.body }}
          submitLabel={m.form.saveChanges}
          cancelLink={
            <Link
              preload={false}
              className={cx('btn btn-quiet')}
              to="/pulls/$pullId"
              params={{ pullId }}
            >
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                await api.updatePull(Number(pullId), {
                  title: input.title,
                  body: input.body,
                })
                await navigate({ to: '/pulls/$pullId', params: { pullId } })
              } catch (err) {
                setErrors(apiFieldErrors(err))
              }
            })
          }}
        />
      </div>
    )
  },
})
