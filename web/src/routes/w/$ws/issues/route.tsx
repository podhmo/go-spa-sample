import { Outlet, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/w/$ws/issues')({
  component: () => <Outlet />,
})
