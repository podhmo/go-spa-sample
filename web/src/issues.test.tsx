import { page } from 'vitest/browser'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import type { Comment, Issue, Label } from './api'
import { IssueForm } from './components/forms'
import { IssueList } from './components/IssueList'
import { renderAt, stubAPI } from './test-utils'

const issue = (over: Partial<Issue> = {}): Issue => ({
  id: 1,
  title: 'first issue',
  body: 'body text',
  status: 'open',
  labels: [],
  author: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...over,
})

const comment = (over: Partial<Comment> = {}): Comment => ({
  id: 1,
  body: 'a comment',
  author: null,
  created_at: '2026-01-01T00:00:00Z',
  hidden: false,
  ...over,
})

const label = (over: Partial<Label> = {}): Label => ({
  id: 1,
  name: 'Bug',
  color: '#d73a4a',
  kind: 'issue',
  workspace_id: null,
  archived: false,
  ...over,
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('component: IssueList', () => {
  it('renders issues and an empty state', async () => {
    render(<IssueList issues={[]} />)
    await expect.element(page.getByText('No issues.')).toBeInTheDocument()
  })
})

describe('component: IssueForm', () => {
  it('submits entered values', async () => {
    const onSubmit = vi.fn()
    render(<IssueForm onSubmit={onSubmit} />)
    await page.getByLabelText(/Title/).fill('hello from test')
    await page.getByRole('button', { name: 'create issue' }).click()
    expect(onSubmit).toHaveBeenCalledWith({ title: 'hello from test', body: '' })
  })
})

describe('route: /issues', () => {
  it('shows issues fetched through the loader', async () => {
    stubAPI((url) => {
      if (url.includes('/labels'))
        return { body: { labels: [label({ name: 'Bug' }), label({ id: 2, name: 'Feature' })] } }
      expect(url).toContain('/api/v1/w/default/issues')
      return {
        body: {
          issues: [issue({ title: 'from api' }), issue({ id: 2, title: 'second' })],
          total: 2,
          page: 1,
          per_page: 20,
        },
      }
    })
    renderAt('/issues')
    await expect.element(page.getByText(/from api/)).toBeInTheDocument()
    await expect.element(page.getByText(/second/)).toBeInTheDocument()
    await expect.element(page.getByText('2 issues (page 1 / 1)')).toBeInTheDocument()
  })

  it('passes search params to the API', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      if (url.includes('/labels')) return { body: { labels: [] } }
      calls.push(url)
      return { body: { issues: [], total: 0, page: 1, per_page: 20 } }
    })
    renderAt('/issues?status=closed&q=bug')
    await expect.element(page.getByText('No issues.')).toBeInTheDocument()
    expect(calls[0]).toContain('status=closed')
    expect(calls[0]).toContain('q=bug')
  })

  it('puts label/sort/page filters in the URL and sends them to the API', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      if (url.includes('/labels'))
        return { body: { labels: [label({ name: 'Bug' }), label({ id: 2, name: 'Feature' })] } }
      calls.push(url)
      return {
        body: {
          issues: [issue({ title: 'filtered' })],
          total: 25,
          page: 2,
          per_page: 20,
        },
      }
    })
    renderAt('/issues?label=Bug&sort=oldest&page=2')
    await expect.element(page.getByText(/filtered/)).toBeInTheDocument()
    expect(calls[0]).toContain('label=Bug')
    expect(calls[0]).toContain('sort=oldest')
    expect(calls[0]).toContain('page=2')
    // label select is populated from the labels endpoint
    await expect.element(page.getByRole('option', { name: 'Feature' })).toBeInTheDocument()
    // page 2 of 2 shows a prev link back to page 1
    await expect.element(page.getByRole('link', { name: 'prev' })).toBeInTheDocument()
  })

  it('navigates to the next page via the pagination link', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      if (url.includes('/labels')) return { body: { labels: [] } }
      calls.push(url)
      return {
        body: { issues: [issue({ title: 'row' })], total: 25, page: 1, per_page: 20 },
      }
    })
    renderAt('/issues')
    await page.getByRole('link', { name: 'next' }).click()
    // the page=2 request proves the link drove a URL-driven loader refetch
    await expect.poll(() => calls.some((u) => u.includes('page=2'))).toBe(true)
    await expect.element(page.getByText(/25 issues \(page 1 \/ 2\)/)).toBeInTheDocument()
  })
})

describe('route: /issues/$issueId', () => {
  it('shows issue and comments, then posts a status change', async () => {
    const requests: { url: string; init?: RequestInit }[] = []
    let closed = false
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.endsWith('/status')) {
        closed = true
        return { body: { issue: issue({ status: 'closed' }) } }
      }
      return {
        body: {
          issue: issue({ title: 'detail page', status: closed ? 'closed' : 'open' }),
          comments: [comment({ body: 'nice work' })],
        },
      }
    })
    renderAt('/issues/1')
    await expect.element(page.getByText(/detail page/)).toBeInTheDocument()
    await expect.element(page.getByText('nice work')).toBeInTheDocument()

    await page.getByRole('button', { name: 'mark closed' }).click()
    await expect.element(page.getByRole('button', { name: 'mark open' })).toBeInTheDocument()
    const statusCall = requests.find((r) => r.url.endsWith('/status'))
    expect(statusCall?.init?.method).toBe('POST')
    expect(JSON.parse(String(statusCall?.init?.body))).toEqual({ status: 'closed' })
  })
})
