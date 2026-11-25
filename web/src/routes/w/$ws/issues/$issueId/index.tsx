import { createFileRoute, Link, redirect, useNavigate, useRouter } from '@tanstack/react-router'
import { useReducer } from 'react'
import { RequestError, api } from '../../../../../api'
import { scoped } from '../../../../-lib/scoped'
import { CommentForm } from '../../../../../components/CommentForm'
import { CommentList, IssueDetail } from './-components/IssueDetail'
import { useMessages } from '../../../../../i18n'
import { cx } from '../../../../../styles/cx'

export const Route = createFileRoute('/w/$ws/issues/$issueId/')({
  loader: ({ params, location }) =>
    scoped(api.getIssue(params.ws, Number(params.issueId)), (real) => {
      throw redirect({
        to: '/w/$ws/issues/$issueId',
        params: { ws: real, issueId: params.issueId },
        hash: location.hash,
      })
    }),
  component: function IssuePage() {
    const { issue, comments } = Route.useLoaderData()
    const { ws, issueId } = Route.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    // A reducer-driven busy flag, not an async transition: renders kicked
    // off by router.invalidate() can suspend on a loader thenable that a
    // later invalidation supersedes, which would pin isPending forever.
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const [actionError, setActionError] = useReducer(
      (_: string | null, v: string | null) => v,
      null,
    )
    const [editError, setEditError] = useReducer(
      (_: { cid: number; message: string } | null, v: { cid: number; message: string } | null) => v,
      null,
    )
    const [commentError, setCommentError] = useReducer(
      (_: string | null, v: string | null) => v,
      null,
    )
    const issueID = Number(issueId)
    const m = useMessages()
    const errorMessage = (err: unknown) => {
      const fields = err instanceof RequestError ? err.apiError?.fields : undefined
      return fields
        ? Object.values(fields).join(' ')
        : err instanceof RequestError
          ? err.message
          : m.error.requestFailed
    }
    const mutate = (fn: () => Promise<unknown>, onError?: (err: unknown) => void) => {
      setPending(true)
      setActionError(null)
      setEditError(null)
      return (async () => {
        try {
          await fn()
          void router.invalidate()
          return true
        } catch (err) {
          // A failed mutation changed nothing; refetching here can drop
          // the whole page into the route error boundary during an outage.
          if (onError) onError(err)
          else setActionError(errorMessage(err))
          return false
        } finally {
          setPending(false)
        }
      })()
    }
    return (
      <div>
        <nav className={cx('breadcrumb')} aria-label={m.chrome.breadcrumb}>
          <ol>
            <li>
              <Link to="/w/$ws/issues" params={{ ws }} activeOptions={{ exact: true }}>
                {m.issues.title}
              </Link>
            </li>
            <li>
              <a href="#" aria-current="page">
                #{issue.id}
              </a>
            </li>
          </ol>
        </nav>

        <IssueDetail
          issue={issue}
          ws={ws}
          pending={isPending}
          onSetStatus={(status) => mutate(() => api.setIssueStatus(ws, issueID, status))}
          onDelete={() => {
            setPending(true)
            void (async () => {
              try {
                await api.deleteIssue(ws, issueID)
                // The page is gone — a dirty comment draft must not
                // trap the post-delete redirect in the leave-guard.
                await navigate({ to: '/w/$ws/issues', params: { ws }, ignoreBlocker: true })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getIssueMarkdown(ws, issueID)}
        />

        <section id="comments" className={cx('comments')}>
          <h2>
            {m.issues.comments}
            {comments.length > 0 && <span className="comment-count"> ({comments.length})</span>}
          </h2>
          {actionError && (
            <em className={cx('error-msg')} role="alert">
              {actionError}
            </em>
          )}
          <CommentList
            comments={comments}
            pending={isPending}
            editError={editError}
            onEditErrorClear={() => setEditError(null)}
            onEdit={(cid, body) =>
              mutate(
                () => api.updateComment(ws, issueID, cid, body),
                (err) => setEditError({ cid, message: errorMessage(err) }),
              )
            }
            onDelete={(cid) => mutate(() => api.deleteComment(ws, issueID, cid))}
            onHide={(cid, reason) => mutate(() => api.hideComment(ws, issueID, cid, reason))}
            onUnhide={(cid) => mutate(() => api.unhideComment(ws, issueID, cid))}
          />
          <CommentForm
            submitting={isPending}
            error={commentError}
            onSubmit={(body) => {
              setCommentError(null)
              return mutate(
                () => api.createComment(ws, issueID, body),
                (err) => setCommentError(errorMessage(err)),
              )
            }}
          />
        </section>

        <a className={cx('back-to-top')} href="#main-content" aria-label={m.chrome.backToTopAria}>
          {m.chrome.backToTop}
        </a>
      </div>
    )
  },
})
