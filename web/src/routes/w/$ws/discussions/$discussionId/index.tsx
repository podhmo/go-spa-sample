import { createFileRoute, Link, redirect, useNavigate, useRouter } from '@tanstack/react-router'
import { useReducer } from 'react'
import { RequestError, api } from '../../../../../api'
import { scoped } from '../../../../-lib/scoped'
import { CommentForm } from '../../../../../components/CommentForm'
import { DiscussionDetail, ThreadList } from './-components/DiscussionDetail'
import { useMessages } from '../../../../../i18n'

export const Route = createFileRoute('/w/$ws/discussions/$discussionId/')({
  loader: ({ params, location }) =>
    scoped(api.getDiscussion(params.ws, Number(params.discussionId)), (real) => {
      throw redirect({
        to: '/w/$ws/discussions/$discussionId',
        params: { ws: real, discussionId: params.discussionId },
        hash: location.hash,
      })
    }),
  component: function DiscussionPage() {
    const { discussion, threads } = Route.useLoaderData()
    const { ws, discussionId } = Route.useParams()
    const router = useRouter()
    const navigate = useNavigate()
    // A reducer-driven busy flag, not an async transition: renders kicked
    // off by router.invalidate() can suspend on a loader thenable that a
    // later invalidation supersedes, which would pin isPending forever.
    const [isPending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
    const did = Number(discussionId)
    const m = useMessages()
    const [error, setError] = useReducer((_: string | null, v: string | null) => v, null)
    const mutate = (fn: () => Promise<unknown>) => {
      setPending(true)
      return (async () => {
        try {
          await fn()
          setError(null)
          void router.invalidate()
          return true
        } catch (err) {
          const fields = err instanceof RequestError ? err.apiError?.fields : undefined
          // Field errors name the actual rejection reason (e.g. a
          // whitespace-only reply); the bare message just says
          // "Validation failed.".
          setError(
            fields
              ? Object.values(fields).join(' ')
              : err instanceof RequestError
                ? err.message
                : m.error.requestFailed,
          )
          return false
        } finally {
          setPending(false)
        }
      })()
    }
    // Posting a thread or reply lands on the new entry's permalink like
    // the MPA's redirect-to-anchor flow.
    const postAndGo = <T,>(fn: () => Promise<T>, anchor: (r: T) => string): Promise<boolean> => {
      setPending(true)
      return (async () => {
        try {
          const created = await fn()
          await router.invalidate()
          void navigate({ to: '.', hash: anchor(created), replace: true })
          // The reloaded list renders after invalidate resolves, so the
          // router's hash scroll may run before the entry exists — retry
          // on the next frame.
          requestAnimationFrame(() => {
            document.getElementById(anchor(created))?.scrollIntoView()
          })
          return true
        } catch {
          // mutation failures surface via the refetched state
          return false
        } finally {
          setPending(false)
        }
      })()
    }
    const answerable = discussion.category.format === 'qa'
    return (
      <div>
        <nav className="breadcrumb" aria-label={m.chrome.breadcrumb}>
          <ol>
            <li>
              <Link to="/w/$ws/discussions" params={{ ws }} activeOptions={{ exact: true }}>
                {m.discussions.title}
              </Link>
            </li>
            <li>
              <a href="#" aria-current="page">
                #{discussion.id}
              </a>
            </li>
          </ol>
        </nav>

        <DiscussionDetail
          discussion={discussion}
          ws={ws}
          pending={isPending}
          onDelete={() => {
            setPending(true)
            void (async () => {
              try {
                await api.deleteDiscussion(ws, did)
                await navigate({
                  to: '/w/$ws/discussions',
                  params: { ws },
                  ignoreBlocker: true,
                })
              } finally {
                setPending(false)
              }
            })()
          }}
          onCopyMarkdown={() => api.getDiscussionMarkdown(ws, did)}
        />

        <section id="comments" className="comments">
          <h2>
            {m.issues.comments}
            {threads.length > 0 && <span className="comment-count"> ({threads.length})</span>}
          </h2>
          <ThreadList
            threads={threads}
            pending={isPending}
            answerable={answerable}
            onEdit={(tid, body) => mutate(() => api.updateThread(ws, did, tid, body))}
            onDelete={(tid) => mutate(() => api.deleteThread(ws, did, tid))}
            onHide={(tid, reason) => mutate(() => api.hideThread(ws, did, tid, reason))}
            onUnhide={(tid) => mutate(() => api.unhideThread(ws, did, tid))}
            onSetAnswer={
              answerable
                ? (tid, answer) => mutate(() => api.setThreadAnswer(ws, did, tid, answer))
                : undefined
            }
            onReply={(tid, body) =>
              postAndGo(
                () => api.createReply(ws, did, tid, body),
                (r) => `comment-${r.reply.id}`,
              )
            }
            replyHandlers={{
              pending: isPending,
              onEdit: (tid, cid, body) => mutate(() => api.updateReply(ws, did, tid, cid, body)),
              onDelete: (tid, cid) => mutate(() => api.deleteReply(ws, did, tid, cid)),
              onHide: (tid, cid, reason) => mutate(() => api.hideReply(ws, did, tid, cid, reason)),
              onUnhide: (tid, cid) => mutate(() => api.unhideReply(ws, did, tid, cid)),
            }}
          />
          {error && <em role="alert">{error}</em>}
          <CommentForm
            submitting={isPending}
            onSubmit={(body) =>
              postAndGo(
                () => api.createThread(ws, did, body),
                (r) => `thread-${r.thread.id}`,
              )
            }
          />
        </section>

        <a className="back-to-top" href="#main-content" aria-label={m.chrome.backToTopAria}>
          {m.chrome.backToTop}
        </a>
      </div>
    )
  },
})
