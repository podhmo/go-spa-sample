import { page } from 'vitest/browser'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type {
  Discussion,
  DiscussionCategory,
  DiscussionReply,
  DiscussionThread,
  Workspace,
} from '../../api'
import { renderAt, stubAPI } from '../../test-utils'

const workspace = (over: Partial<Workspace> = {}): Workspace => ({
  id: 1,
  slug: 'default',
  name: 'default',
  description: '',
  issue_count: 0,
  discussion_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  ...over,
})

const cat = (over: Partial<DiscussionCategory> = {}): DiscussionCategory => ({
  id: 2,
  name: 'General',
  slug: 'general',
  description: 'Anything',
  format: 'open',
  ...over,
})

const qaCat = cat({ id: 4, name: 'Q&A', slug: 'qa', format: 'qa' })

const discussion = (over: Partial<Discussion> = {}): Discussion => ({
  id: 1,
  title: 'first discussion',
  body: 'disc body',
  category: cat(),
  labels: [],
  author: null,
  answered: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...over,
})

const thread = (over: Partial<DiscussionThread> = {}): DiscussionThread => ({
  id: 1,
  is_answer: false,
  body: 'op comment',
  author: null,
  created_at: '2026-01-01T00:00:00Z',
  comments: [],
  hidden: false,
  ...over,
})

const reply = (over: Partial<DiscussionReply> = {}): DiscussionReply => ({
  id: 1,
  thread_id: 1,
  body: 'nested reply',
  author: null,
  created_at: '2026-01-01T00:00:00Z',
  hidden: false,
  ...over,
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('route: /w/{ws}/discussions', () => {
  it('lists discussions and forwards the filters', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      if (url.includes('/discussion-categories')) return { body: { categories: [cat(), qaCat] } }
      if (url.includes('/pulls')) return { status: 404, body: {} } // availability probe
      calls.push(url)
      return {
        body: {
          discussions: [discussion({ title: 'from api' })],
          total: 1,
          page: 1,
          per_page: 20,
        },
      }
    })
    const { router } = renderAt('/w/default/discussions?category=qa&answered=yes')
    await expect.element(page.getByText(/from api/)).toBeInTheDocument()
    expect(calls[0]).toContain('category=qa')
    expect(calls[0]).toContain('answered=yes')
    expect(router.state.location.pathname).toBe('/w/default/discussions')
  })

  it('redirects the workspace-less /discussions URL', async () => {
    stubAPI((url) => {
      if (url.includes('/discussion-categories')) return { body: { categories: [cat()] } }
      if (url.endsWith('/api/v1/workspaces')) return { body: { workspaces: [workspace()] } }
      return { body: { discussions: [discussion()], total: 1, page: 1, per_page: 20 } }
    })
    const { router } = renderAt('/discussions')
    await expect.element(page.getByRole('heading', { name: 'Discussions' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/w/default/discussions')
  })
})

describe('route: /w/{ws}/discussions/{id}', () => {
  it('renders the discussion with threads and a nested reply', async () => {
    stubAPI((url) => {
      if (url.includes('/pulls')) return { status: 404, body: {} } // availability probe
      expect(url).toContain('/w/default/discussions/1')
      return {
        body: {
          discussion: discussion({ category: qaCat, answered: true }),
          threads: [thread({ is_answer: true, comments: [reply()] })],
        },
      }
    })
    renderAt('/w/default/discussions/1')
    await expect
      .element(page.getByRole('heading', { name: /first discussion/ }))
      .toBeInTheDocument()
    await expect.element(page.getByText('op comment')).toBeInTheDocument()
    await expect.element(page.getByText('nested reply')).toBeInTheDocument()
    await expect.element(page.getByText('Q&A').first()).toBeInTheDocument()
  })

  it('shows Mark as answer only for qa categories', async () => {
    stubAPI(() => ({
      body: { discussion: discussion({ category: qaCat }), threads: [thread()] },
    }))
    renderAt('/w/default/discussions/1')
    await expect.element(page.getByRole('button', { name: 'Mark as answer' })).toBeInTheDocument()
  })

  it('hides Mark as answer on open categories', async () => {
    stubAPI(() => ({
      body: { discussion: discussion(), threads: [thread()] },
    }))
    renderAt('/w/default/discussions/1')
    await expect
      .element(page.getByRole('heading', { name: /first discussion/ }))
      .toBeInTheDocument()
    expect(page.getByRole('button', { name: 'Mark as answer' }).elements()).toHaveLength(0)
  })

  it('keeps a collapsed reply draft and restores it on reopen', async () => {
    stubAPI(() => ({
      body: { discussion: discussion(), threads: [thread()] },
    }))
    renderAt('/w/default/discussions/1')
    await expect.element(page.getByText('op comment')).toBeInTheDocument()
    const toggle = page.getByRole('button', { name: 'Reply' }).first()
    await toggle.click()
    const box = page.getByPlaceholder('Reply…')
    await box.fill('half-written reply')
    await toggle.click()
    expect(page.getByPlaceholder('Reply…').elements()).toHaveLength(0)
    await toggle.click()
    await expect.element(page.getByPlaceholder('Reply…')).toHaveValue('half-written reply')
  })

  it('clears the reply draft after a successful post', async () => {
    stubAPI((url, init) => {
      if (init?.method === 'POST' && url.includes('/comments')) {
        return { body: { reply: reply({ id: 9 }) } }
      }
      return { body: { discussion: discussion(), threads: [thread()] } }
    })
    renderAt('/w/default/discussions/1')
    await expect.element(page.getByText('op comment')).toBeInTheDocument()
    const toggle = page.getByRole('button', { name: 'Reply' }).first()
    await toggle.click()
    await page.getByPlaceholder('Reply…').fill('done reply')
    await page.getByRole('button', { name: 'Reply', exact: true }).last().click()
    // Wait for the round trip (post + invalidate) instead of a fixed delay.
    await expect.poll(() => page.getByPlaceholder('Reply…').elements().length).toBe(0)
    await toggle.click()
    await expect.element(page.getByPlaceholder('Reply…')).toHaveValue('')
  })

  it('guards a collapsed reply draft: declining keeps the text, accepting navigates', async () => {
    stubAPI((url) => {
      if (url.includes('/pulls')) return { status: 404, body: {} }
      if (url.endsWith('/discussions') || url.includes('/discussions?')) {
        return { body: { discussions: [], total: 0, page: 1, per_page: 20 } }
      }
      return { body: { discussion: discussion(), threads: [thread()] } }
    })
    renderAt('/w/default/discussions/1')
    await expect.element(page.getByText('op comment')).toBeInTheDocument()
    const toggle = page.getByRole('button', { name: 'Reply' }).first()
    await toggle.click()
    await page.getByPlaceholder('Reply…').fill('collapsed draft')
    await toggle.click() // collapse — the draft must still be guarded
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await page.getByRole('link', { name: 'Discussions' }).first().click()
    await expect.poll(() => confirm.mock.calls.length > 0).toBeTruthy()
    await toggle.click()
    await expect.element(page.getByPlaceholder('Reply…')).toHaveValue('collapsed draft')
    confirm.mockRestore()
  })

  it('keeps the reply form open with its draft when the post is rejected', async () => {
    let posts = 0
    stubAPI((url, init) => {
      if (init?.method === 'POST' && url.includes('/comments')) {
        posts += 1
        return { status: 500, body: { error: { message: 'boom' } } }
      }
      return { body: { discussion: discussion(), threads: [thread()] } }
    })
    renderAt('/w/default/discussions/1')
    await expect.element(page.getByText('op comment')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Reply' }).first().click()
    await page.getByPlaceholder('Reply…').fill('do not lose me')
    await page.getByRole('button', { name: 'Reply', exact: true }).last().click()
    // The rejection must actually have happened — otherwise the next
    // assertion passes vacuously.
    await expect.poll(() => posts).toBe(1)
    // The form stays open with the text intact — no silent close.
    await expect.element(page.getByPlaceholder('Reply…')).toHaveValue('do not lose me')
  })
})
