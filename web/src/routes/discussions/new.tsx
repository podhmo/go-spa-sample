import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../-lib/scoped'

export const Route = createFileRoute('/discussions/new')({
  beforeLoad: async () => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/discussions/new', params: { ws } } : { to: '/workspaces' })
  },
})
