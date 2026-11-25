import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useReducer, useTransition } from 'react'
import { api } from '../../api'
import { apiFieldErrors } from '../-lib/mutation'
import { PullForm } from './-components/PullForm'
import { pullOrNotFound } from './-lib/pull-or-not-found'
import { useMessages } from '../../i18n'
import { cx } from '../../styles/cx'

export const Route = createFileRoute('/pulls/new')({
  loader: () => pullOrNotFound(api.listPullRefs()),
  component: function NewPullPage() {
    const { refs } = Route.useLoaderData()
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
          <h1>{m.pulls.new}</h1>
        </div>
        <PullForm
          submitting={isPending}
          errors={errors}
          refs={refs}
          cancelLink={
            <Link className={cx('btn btn-quiet')} to="/pulls">
              {m.copy.cancel}
            </Link>
          }
          onSubmit={(input) => {
            startTransition(async () => {
              try {
                const { pull_request } = await api.createPull({
                  title: input.title,
                  body: input.body,
                  base: input.base ?? '',
                  head: input.head ?? '',
                })
                await navigate({
                  to: '/pulls/$pullId',
                  params: { pullId: String(pull_request.id) },
                })
              } catch (err) {
                setErrors(apiFieldErrors(err, 'head'))
              }
            })
          }}
        />
      </div>
    )
  },
})
