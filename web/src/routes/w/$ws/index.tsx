import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/w/$ws/')({
  beforeLoad: ({ params }) => {
    throw redirect({ to: '/w/$ws/issues', params: { ws: params.ws } })
  },
})
