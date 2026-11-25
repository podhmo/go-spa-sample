import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { api } from '../../../../api'
import { apiFieldErrors } from '../../../-lib/mutation'
import { scoped } from '../../../-lib/scoped'
import { DiscussionForm } from './-components/DiscussionForm'
import { useMessages } from '../../../../i18n'

export const Route = createFileRoute('/w/$ws/discussions/new')({
  loader: async ({ params }) => {
    const [categories, labels] = await scoped(
      Promise.all([
        api.listDiscussionCategories(),
        api.listLabels(params.ws, { kind: 'discussion' }),
      ]),
      (real) => {
        throw redirect({ to: '/w/$ws/discussions/new', params: { ws: real } })
      },
    )
    return { categories: categories.categories, labels: labels.labels }
  },
  component: function NewDiscussionPage() {
    const { categories, labels } = Route.useLoaderData()
    const { ws } = Route.useParams()
    const navigate = useNavigate()
    const [isPending, startTransition] = useTransition()
    const [errors, setErrors] = useReducer(
      (_: Record<string, string>, e: Record<string, string>) => e,
      {},
    )
    const m = useMessages()
    return (
      <div>
        <div className="page-head">
          <h1>{m.discussions.new}</h1>
        </div>
        <DiscussionForm
          submitting={isPending}
          errors={errors}
          categories={categories}
          labels={labels}
          cancelLink={
            <Link className="btn btn-quiet" to="/w/$ws/discussions" params={{ ws }}>
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { discussion } = await api.createDiscussion(ws, input)
                await navigate({
                  to: '/w/$ws/discussions/$discussionId',
                  params: { ws, discussionId: String(discussion.id) },
                })
              } catch (err) {
                setErrors(apiFieldErrors(err, 'category'))
              }
            })
          }}
        />
      </div>
    )
  },
})
