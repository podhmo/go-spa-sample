import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../../-lib/scoped'

export const Route = createFileRoute('/discussions/$discussionId/')({
  beforeLoad: async ({ params, location }) => {
    const ws = await homeSlug()
    // #thread-N / #comment-N permalinks survive the redirect.
    throw redirect(
      ws
        ? {
            to: '/w/$ws/discussions/$discussionId',
            params: { ws, discussionId: params.discussionId },
            hash: location.hash,
          }
        : { to: '/workspaces' },
    )
  },
})
