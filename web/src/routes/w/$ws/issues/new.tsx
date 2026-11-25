import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { api } from '../../../../api'
import { apiFieldErrors } from '../../../-lib/mutation'
import { scoped } from '../../../-lib/scoped'
import { listIssueFormLabels } from './-data'
import { IssueForm } from './-components/IssueForm'
import { useMessages } from '../../../../i18n'
import { cx } from '../../../../styles/cx'

export const Route = createFileRoute('/w/$ws/issues/new')({
  loader: ({ params }) =>
    scoped(listIssueFormLabels(params.ws), (real) => {
      throw redirect({ to: '/w/$ws/issues/new', params: { ws: real } })
    }),
  component: function NewIssuePage() {
    const { labels } = Route.useLoaderData()
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
        <div className={cx('page-head')}>
          <h1>{m.issues.new}</h1>
        </div>
        <IssueForm
          submitting={isPending}
          errors={errors}
          labels={labels}
          cancelLink={
            <Link className={cx('btn btn-quiet')} to="/w/$ws/issues" params={{ ws }}>
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { issue } = await api.createIssue(ws, input)
                await navigate({
                  to: '/w/$ws/issues/$issueId',
                  params: { ws, issueId: String(issue.id) },
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
