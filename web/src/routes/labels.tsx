import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from './-lib/scoped'

export const Route = createFileRoute('/labels')({
  beforeLoad: async () => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/labels', params: { ws } } : { to: '/workspaces' })
  },
})
