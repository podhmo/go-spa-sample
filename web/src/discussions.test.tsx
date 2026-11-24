import { page } from 'vitest/browser'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type {
  Discussion,
  DiscussionCategory,
  DiscussionReply,
  DiscussionThread,
  Workspace,
} from './api'
import { DiscussionForm } from './components/forms'
import { renderAt, renderWithRouter, stubAPI } from './test-utils'

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

describe('component: DiscussionForm', () => {
  it('requires a category and submits it', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<DiscussionForm onSubmit={onSubmit} categories={[cat(), qaCat]} />)
    await page.getByLabelText(/Title/).fill('hello discussion')
    await page.getByLabelText(/Category/).selectOptions('4')
    await page.getByRole('button', { name: 'Create discussion' }).click()
    expect(onSubmit).toHaveBeenCalledWith({
      title: 'hello discussion',
      body: '',
      category_id: 4,
      label_ids: [],
    })
  })

  it('rejects submit without a category', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<DiscussionForm onSubmit={onSubmit} categories={[cat(), qaCat]} />)
    await page.getByLabelText(/Title/).fill('hello discussion')
    await page.getByRole('button', { name: 'Create discussion' }).click()
    expect(onSubmit).not.toHaveBeenCalled()
  })
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
})
