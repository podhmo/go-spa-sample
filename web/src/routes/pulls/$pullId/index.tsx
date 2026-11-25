import { createFileRoute, Link, useNavigate, useRouter } from '@tanstack/react-router'
import { useReducer } from 'react'
import { RequestError, api } from '../../../api'
import { getPullOrNotFound } from '../-lib/pull-or-not-found'
import { CommentForm } from '../../../components/CommentForm'
import { PRCommentList, PullCommitList, PullDetail } from './-components/PullDetail'
import { useMessages } from '../../../i18n'
import { cx } from '../../../styles/cx'

export const Route = createFileRoute('/pulls/$pullId/')({
  loader: ({ params }) => getPullOrNotFound(Number(params.pullId)),
  component: function PullPage() {
    const { pull_request, comments, commits, mergeable } = Route.useLoaderData()
    const { pullId } = Route.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [actionError, setActionError] = useReducer(
      (_: string | null, v: string | null) => v,
      null,
    )
    const pid = Number(pullId)
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      setActionError(null)
      return (async () => {
        try {
          await fn()
          void router.invalidate()
          return true
        } catch (err) {
          // A failed mutation changes nothing; refetching here can drop
          // the whole page into the route error boundary during an outage.
          setActionError(err instanceof RequestError ? err.message : m.error.requestFailed)
          return false
        } finally {
          setPending(false)
        }
      })()
    }
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
              <a href="#" aria-current="page">
                #{pull_request.id}
              </a>
            </li>
          </ol>
        </nav>

        <PullDetail
          pull={pull_request}
          mergeable={mergeable}
          pending={isPending}
          actionError={actionError}
          onSetStatus={(status) => mutate(() => api.setPullStatus(pid, status))}
          onMerge={(method, message) =>
            mutate(() => api.mergePull(pid, method, message || undefined))
          }
          onDelete={() => {
            setPending(true)
            void (async () => {
              try {
                await api.deletePull(pid)
                await navigate({ to: '/pulls', ignoreBlocker: true })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getPullMarkdown(pid)}
        />

        <PullCommitList commits={commits} />

        <section id="comments" className="comments">
          <h2>
            {m.issues.comments}
            {comments.length > 0 && <span className="comment-count"> ({comments.length})</span>}
          </h2>
          <PRCommentList
            comments={comments}
            pending={isPending}
            onEdit={(cid, body) => mutate(() => api.updatePRComment(pid, cid, body))}
            onDelete={(cid) => mutate(() => api.deletePRComment(pid, cid))}
            onHide={(cid, reason) => mutate(() => api.hidePRComment(pid, cid, reason))}
            onUnhide={(cid) => mutate(() => api.unhidePRComment(pid, cid))}
          />
          <CommentForm
            submitting={isPending}
            onSubmit={(body) => mutate(() => api.createPRComment(pid, { body }))}
          />
        </section>

        <a className="back-to-top" href="#main-content" aria-label={m.chrome.backToTopAria}>
          {m.chrome.backToTop}
        </a>
      </div>
    )
  },
})
