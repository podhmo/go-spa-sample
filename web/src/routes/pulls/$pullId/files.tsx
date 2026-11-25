import { createFileRoute, Link, useNavigate, useRouter } from '@tanstack/react-router'
import { useReducer } from 'react'
import { RequestError, api, type PRCommentInput } from '../../../api'
import { getPullFilesOrNotFound } from '../-lib/pull-or-not-found'
import { PullFiles } from './-components/PullFiles'
import { useMessages } from '../../../i18n'
import styles from './-components/PullFiles.module.css'
import { cx } from '../../../styles/cx'

export const Route = createFileRoute('/pulls/$pullId/files')({
  loader: ({ params }) => getPullFilesOrNotFound(Number(params.pullId)),
  component: function PullFilesPage() {
    const { pull_request, diff } = Route.useLoaderData()
    const { text: diffText, head_sha: headSha, truncated } = diff
    const { pullId } = Route.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const m = useMessages()
    return (
      <div>
        <nav className={cx('breadcrumb')} aria-label={m.chrome.breadcrumb}>
          <ol>
            <li>
              <Link to="/pulls" activeOptions={{ exact: true }}>
                {m.pulls.title}
              </Link>
            </li>
            <li>
              <Link preload={false} to="/pulls/$pullId" params={{ pullId }}>
                #{pull_request.id}
              </Link>
            </li>
            <li>
              <a href="#" aria-current="page">
                {m.pulls.files}
              </a>
            </li>
          </ol>
        </nav>

        <div className={cx('page-head')}>
          <h1>
            {pull_request.title} <span className="issue-num">#{pull_request.id}</span>
          </h1>
          <Link preload={false} className={cx('btn')} to="/pulls/$pullId" params={{ pullId }}>
            {m.pulls.conversation}
          </Link>
        </div>

        {truncated && (
          <p className={styles.notice} role="status">
            {m.pulls.truncated}
          </p>
        )}
        <PullFiles
          diff={diffText}
          pending={isPending}
          onComment={(input: PRCommentInput) => {
            setPending(true)
            return (async () => {
              try {
                const { comment } = await api.createPRComment(Number(pullId), {
                  ...input,
                  anchor: headSha,
                })
                // Invalidate before navigating so the destination never
                // serves the pre-mutation loader snapshot (ui-rules §13).
                void router.invalidate()
                await navigate({
                  to: '/pulls/$pullId',
                  params: { pullId },
                  hash: `comment-${comment.id}`,
                })
              } catch (err) {
                // Stale anchor/path errors need a fresh diff. Network/5xx
                // failures must not refetch: the loader failure would
                // replace the page — and the preserved draft — with the
                // route error boundary.
                if (
                  err instanceof RequestError &&
                  (err.apiError?.fields?.anchor !== undefined ||
                    err.apiError?.fields?.path !== undefined)
                ) {
                  void router.invalidate()
                }
                throw err
              } finally {
                setPending(false)
              }
            })()
          }}
        />
      </div>
    )
  },
})
