import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { api } from '../../../../../api'
import { listIssueFormLabels } from '../-data'
import { IssueForm } from '../-components/IssueForm'
import { apiFieldErrors } from '../../../../-lib/mutation'
import { scoped } from '../../../../-lib/scoped'
import { fmt, useMessages } from '../../../../../i18n'
import { cx } from '../../../../../styles/cx'

export const Route = createFileRoute('/w/$ws/issues/$issueId/edit')({
  loader: async ({ params }) => {
    const rebase = (real: string) => {
      throw redirect({
        to: '/w/$ws/issues/$issueId/edit',
        params: { ws: real, issueId: params.issueId },
      })
    }
    const [detail, labels] = await scoped(
      Promise.all([
        api.getIssue(params.ws, Number(params.issueId)),
        listIssueFormLabels(params.ws),
      ]),
      rebase,
    )
    return { ...detail, labels: labels.labels }
  },
  component: function EditIssuePage() {
    const { issue, labels } = Route.useLoaderData()
    const { ws, issueId } = Route.useParams()
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
          <h1>{fmt(m.issues.editTitle, { id: issue.id })}</h1>
        </div>
        <IssueForm
          submitting={isPending}
          errors={errors}
          labels={labels}
          initial={{
            title: issue.title,
            body: issue.body,
            label_ids: issue.labels.map((l) => l.id),
          }}
          submitLabel={m.form.saveChanges}
          cancelLink={
            <Link
              className={cx('btn btn-quiet')}
              to="/w/$ws/issues/$issueId"
              params={{ ws, issueId }}
            >
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                await api.updateIssue(ws, Number(issueId), input)
                await navigate({
                  to: '/w/$ws/issues/$issueId',
                  params: { ws, issueId },
                })
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
