import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../../-lib/scoped'

export const Route = createFileRoute('/issues/$issueId/')({
  beforeLoad: async ({ params, location }) => {
    const ws = await homeSlug()
    // location.hash keeps a #comment-N permalink across the redirect.
    throw redirect(
      ws
        ? {
            to: '/w/$ws/issues/$issueId',
            params: { ws, issueId: params.issueId },
            hash: location.hash,
          }
        : { to: '/workspaces' },
    )
  },
})
