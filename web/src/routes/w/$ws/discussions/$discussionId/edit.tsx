import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { api } from '../../../../../api'
import { listDiscussionFormData } from '../-data'
import { DiscussionForm } from '../-components/DiscussionForm'
import { apiFieldErrors } from '../../../../-lib/mutation'
import { scoped } from '../../../../-lib/scoped'
import { fmt, useMessages } from '../../../../../i18n'

export const Route = createFileRoute('/w/$ws/discussions/$discussionId/edit')({
  loader: async ({ params }) => {
    const [detail, formData] = await scoped(
      Promise.all([
        api.getDiscussion(params.ws, Number(params.discussionId)),
        listDiscussionFormData(params.ws),
      ]),
      (real) => {
        throw redirect({
          to: '/w/$ws/discussions/$discussionId/edit',
          params: { ws: real, discussionId: params.discussionId },
        })
      },
    )
    return { ...detail, ...formData }
  },
  component: function EditDiscussionPage() {
    const { discussion, labels, categories } = Route.useLoaderData()
    const { ws, discussionId } = Route.useParams()
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
          <h1>{fmt(m.discussions.editTitle, { id: discussion.id })}</h1>
        </div>
        <DiscussionForm
          submitting={isPending}
          errors={errors}
          categories={categories}
          labels={labels}
          initial={{
            title: discussion.title,
            body: discussion.body,
            category_id: discussion.category.id,
            label_ids: discussion.labels.map((l) => l.id),
          }}
          submitLabel={m.form.saveChanges}
          cancelLink={
            <Link
              className="btn btn-quiet"
              to="/w/$ws/discussions/$discussionId"
              params={{ ws, discussionId }}
            >
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                await api.updateDiscussion(ws, Number(discussionId), input)
                await navigate({
                  to: '/w/$ws/discussions/$discussionId',
                  params: { ws, discussionId },
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
