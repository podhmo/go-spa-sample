import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../-lib/scoped'
import { issueSearch } from '../-lib/search'

export const Route = createFileRoute('/issues/')({
  validateSearch: issueSearch,
  beforeLoad: async ({ search }) => {
    const ws = await homeSlug()
    throw redirect(ws ? { to: '/w/$ws/issues', params: { ws }, search } : { to: '/workspaces' })
  },
})
