import { afterEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { stubAPI, renderAt } from './test-utils'

// The fetch stub can't model the cookie jar, so `loggedIn` plays the
// session's role: /auth/me answers {user} only while it is set, and the
// login/register/logout endpoints flip it.
function stubAuth(
  extra?: (url: string, init?: RequestInit) => { status?: number; body?: unknown } | undefined,
) {
  const state = { loggedIn: false }
  const user = { id: 1, name: 'alice' }
  stubAPI(
    (url, init) => {
      if (url.endsWith('/api/v1/auth/register') && init?.method === 'POST') {
        const { name, password } = JSON.parse(String(init.body))
        if (name === 'taken') {
          return {
            status: 422,
            body: {
              error: { message: 'Validation failed.', fields: { name: 'Name is already taken.' } },
            },
          }
        }
        if (String(password).length < 8) {
          return {
            status: 422,
            body: {
              error: {
                message: 'Validation failed.',
                fields: { password: 'Password must be at least 8 characters.' },
              },
            },
          }
        }
        state.loggedIn = true
        return { status: 201, body: { user } }
      }
      if (url.endsWith('/api/v1/auth/login') && init?.method === 'POST') {
        const { name, password } = JSON.parse(String(init.body))
        if (name === 'alice' && password === 'password1') {
          state.loggedIn = true
          return { body: { user } }
        }
        return { status: 422, body: { error: { message: 'Invalid name or password.' } } }
      }
      if (url.endsWith('/api/v1/auth/logout') && init?.method === 'POST') {
        // The real endpoint's CSRF gate rejects non-JSON posts — enforce
        // it here so the button can't silently drop the header.
        // Mirror requireJSON exactly: only `application/json` (or
        // `application/json;…`), case-insensitive — `application/jsonp`
        // is rejected server-side too.
        const ct = String(new Headers(init.headers).get('content-type') ?? '').toLowerCase()
        if (ct !== 'application/json' && !ct.startsWith('application/json;')) {
          return {
            status: 415,
            body: { error: { message: 'Content-Type must be application/json.' } },
          }
        }
        state.loggedIn = false
        return { status: 204 }
      }
      // A `default` workspace so / and /issues resolve to /w/default/...
      return (
        extra?.(url, init) ?? {
          body: {
            workspaces: [
              {
                id: 1,
                slug: 'default',
                name: 'Default',
                description: '',
                issue_count: 0,
                discussion_count: 0,
                created_at: '2026-01-01T00:00:00Z',
              },
            ],
            issues: [],
            labels: [],
          },
        }
      )
    },
    { me: () => (state.loggedIn ? { body: { user } } : { status: 401, body: {} }) },
  )
  return state
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('route: /register', () => {
  it('registers, then lands on next already signed in', async () => {
    stubAuth()
    renderAt('/register?next=%2Fissues')
    await page.getByLabelText(/Name/).fill('alice')
    await page.getByLabelText(/Password/).fill('password1')
    await page.getByRole('button', { name: 'Register' }).click()
    // Signed in → nav greets the user, and /issues is the landing page.
    await expect.element(page.getByText('alice')).toBeInTheDocument()
    await expect.element(page.getByRole('heading', { name: 'Issues' })).toBeInTheDocument()
  })

  it('shows the field error for a taken name', async () => {
    stubAuth()
    renderAt('/register')
    await page.getByLabelText(/Name/).fill('taken')
    await page.getByLabelText(/Password/).fill('password1')
    await page.getByRole('button', { name: 'Register' }).click()
    await expect.element(page.getByText('Name is already taken.')).toBeInTheDocument()
  })

  it('refuses an off-site next and lands on /', async () => {
    stubAuth()
    renderAt('/register?next=%2F%2Fevil.example')
    await page.getByLabelText(/Name/).fill('alice')
    await page.getByLabelText(/Password/).fill('password1')
    await page.getByRole('button', { name: 'Register' }).click()
    // '//evil.example' fails safeNext → '/'; the workspace picker shows.
    await expect.element(page.getByRole('heading', { name: 'Issues' })).toBeInTheDocument()
  })
})

describe('route: /login', () => {
  it('shows the vague credential error without field hints', async () => {
    stubAuth()
    renderAt('/login')
    await page.getByLabelText(/Name/).fill('alice')
    await page.getByLabelText(/Password/).fill('wrong-password')
    await page.getByRole('button', { name: 'Log in' }).click()
    await expect.element(page.getByText('Invalid name or password.')).toBeInTheDocument()
    // Still on the login form.
    await expect.element(page.getByRole('button', { name: 'Log in' })).toBeInTheDocument()
  })

  it('logs in and navigates to next', async () => {
    stubAuth()
    renderAt('/login?next=%2Fissues')
    await page.getByLabelText(/Name/).fill('alice')
    await page.getByLabelText(/Password/).fill('password1')
    await page.getByRole('button', { name: 'Log in' }).click()
    await expect.element(page.getByRole('heading', { name: 'Issues' })).toBeInTheDocument()
    await expect.element(page.getByText('alice')).toBeInTheDocument()
  })
})

describe('auth nav', () => {
  it('bounces a signed-in user off /login', async () => {
    const state = stubAuth()
    state.loggedIn = true
    renderAt('/login')
    await expect.element(page.getByRole('heading', { name: 'Issues' })).toBeInTheDocument()
  })

  it('logs out and returns the nav to anonymous', async () => {
    const state = stubAuth()
    state.loggedIn = true
    renderAt('/')
    await expect.element(page.getByText('alice')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Log out' }).click()
    await expect.element(page.getByRole('link', { name: 'Log in' }).first()).toBeInTheDocument()
    await expect.poll(() => state.loggedIn).toBe(false)
  })
})
