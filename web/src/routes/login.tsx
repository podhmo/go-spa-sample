import { createFileRoute, redirect } from '@tanstack/react-router'
import { api } from '../api'
import { authSearch, safeNext } from './-lib/search'
import { AuthPage } from './-auth/AuthPage'

export const Route = createFileRoute('/login')({
  validateSearch: authSearch,
  beforeLoad: async ({ search }) => {
    if (await api.me()) throw redirect({ href: safeNext(search.next) })
  },
  component: function LoginPage() {
    const { next } = Route.useSearch()
    return <AuthPage mode="login" next={next} />
  },
})
