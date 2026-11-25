import { Outlet, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/w/$ws/discussions')({
  component: () => <Outlet />,
})
