import { createFileRoute, redirect } from '@tanstack/react-router'
import { homeSlug } from '../-lib/scoped'
import { discussionSearch } from '../-lib/search'

export const Route = createFileRoute('/discussions/')({
  validateSearch: discussionSearch,
  beforeLoad: async ({ search }) => {
    const ws = await homeSlug()
    throw redirect(
      ws ? { to: '/w/$ws/discussions', params: { ws }, search } : { to: '/workspaces' },
    )
  },
})
