import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../-lib/scoped'

export const Route = createFileRoute('/issues/new')({
  beforeLoad: async () => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/issues/new', params: { ws } } : { to: '/workspaces' })
  },
})
