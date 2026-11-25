import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../../-lib/scoped'

export const Route = createFileRoute('/discussions/$discussionId/edit')({
  beforeLoad: async ({ params }) => {
    const ws = await homeSlug()
    throw redirect(
      ws
        ? {
            to: '/w/$ws/discussions/$discussionId/edit',
            params: { ws, discussionId: params.discussionId },
          }
        : { to: '/workspaces' },
    )
  },
})
