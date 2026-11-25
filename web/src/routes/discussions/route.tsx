import { Outlet, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/discussions')({
  component: () => <Outlet />,
})
