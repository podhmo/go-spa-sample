import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../../-lib/scoped'

export const Route = createFileRoute('/issues/$issueId/edit')({
  beforeLoad: async ({ params }) => {
    const ws = await homeSlug()
    throw redirect(
      ws
        ? { to: '/w/$ws/issues/$issueId/edit', params: { ws, issueId: params.issueId } }
        : { to: '/workspaces' },
    )
  },
})
