import { page } from 'vitest/browser'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { PRComment, PRCommit, PullRequest } from './api'
import { PullForm } from './components/forms'
import { renderAt, renderWithRouter, stubAPI } from './test-utils'

const pull = (over: Partial<PullRequest> = {}): PullRequest => ({
  id: 1,
  title: 'first pull request',
  body: 'pr body',
  status: 'open',
  base: 'refs/heads/main',
  head: 'refs/heads/feature-x',
  author: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...over,
})

const prComment = (over: Partial<PRComment> = {}): PRComment => ({
  id: 1,
  body: 'a review comment',
  author: null,
  created_at: '2026-01-01T00:00:00Z',
  hidden: false,
  ...over,
})

const prCommit = (over: Partial<PRCommit> = {}): PRCommit => ({
  sha: 'abcdef1234567890',
  subject: 'add feature x',
  author: 'dev',
  date: '2026-01-01T00:00:00Z',
  ...over,
})

const refs = {
  refs: [
    { name: 'refs/heads/main', short: 'main' },
    { name: 'refs/heads/feature-x', short: 'feature-x' },
  ],
}

// The root loader probes /api/v1/pulls?per_page=1 for availability; every
// stub here answers it with a real list so the Pulls nav link renders.
const probe = (url: string) => url.includes('/api/v1/pulls?per_page=1')

const DIFF = [
  'diff --git a/a.txt b/a.txt',
  'index 111..222 100644',
  '--- a/a.txt',
  '+++ b/a.txt',
  '@@ -1,2 +1,3 @@',
  ' ctx line',
  '-removed line',
  '+added line',
  '+another add',
].join('\n')

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('component: PullForm', () => {
  it('requires branches and a title, then submits refs', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<PullForm onSubmit={onSubmit} refs={refs.refs} />)
    await page.getByLabelText(/Head/).selectOptions('refs/heads/feature-x')
    await page.getByLabelText(/Base/).selectOptions('refs/heads/main')
    await page.getByLabelText(/Title/).fill('my pr')
    await page.getByRole('button', { name: 'Create pull request' }).click()
    expect(onSubmit).toHaveBeenCalledWith({
      title: 'my pr',
      body: '',
      base: 'refs/heads/main',
      head: 'refs/heads/feature-x',
    })
  })

  it('edits title and body only', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<PullForm onSubmit={onSubmit} initial={{ title: 'old', body: 'old body' }} />)
    await page.getByLabelText(/Title/).fill('renamed')
    await page.getByRole('button', { name: 'Save changes' }).click()
    expect(onSubmit).toHaveBeenCalledWith({ title: 'renamed', body: 'old body' })
  })
})

describe('route: /pulls', () => {
  it('lists pull requests and forwards the filters', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      calls.push(url)
      return { body: { pulls: [pull()], total: 1, page: 1, per_page: 20 } }
    })
    renderAt('/pulls?status=merged&q=widget')
    await expect.element(page.getByText(/first pull request/)).toBeInTheDocument()
    const listCall = calls.find((u) => !probe(u))
    expect(listCall).toContain('status=merged')
    expect(listCall).toContain('q=widget')
  })

  it('renders the empty state and the feature-x → main refs', async () => {
    stubAPI(() => ({ body: { pulls: [pull()], total: 1, page: 1, per_page: 20 } }))
    renderAt('/pulls')
    await expect.element(page.getByText('feature-x')).toBeInTheDocument()
    await expect.element(page.getByText('main')).toBeInTheDocument()
    await expect.element(page.getByText('open').first()).toBeInTheDocument()
  })

  it('404s when the server has no git repo', async () => {
    stubAPI(() => ({ status: 404, body: { error: { message: 'Not found.' } } }))
    renderAt('/pulls')
    await expect.element(page.getByText(/404 — not found/)).toBeInTheDocument()
  })
})

describe('route: /pulls/{id}', () => {
  const stubDetail = (over: Partial<PullRequest> = {}) =>
    stubAPI((url) => {
      if (url.endsWith('/diff')) return { body: DIFF }
      return {
        body: {
          pull_request: pull(over),
          comments: [
            prComment(),
            prComment({ id: 2, body: 'line note', path: 'a.txt', line: 3, side: 'right' }),
          ],
          commits: [prCommit()],
          mergeable: { status: 'clean' },
        },
      }
    })

  it('renders status, refs, mergeability, commits and comments', async () => {
    stubDetail()
    renderAt('/pulls/1')
    await expect
      .element(page.getByRole('heading', { name: /first pull request/ }))
      .toBeInTheDocument()
    await expect.element(page.getByText('a review comment')).toBeInTheDocument()
    await expect.element(page.getByText('line note')).toBeInTheDocument()
    await expect.element(page.getByText('a.txt:3')).toBeInTheDocument()
    await expect.element(page.getByText('add feature x')).toBeInTheDocument()
    await expect.element(page.getByText('mergeable')).toBeInTheDocument()
    await expect.element(page.getByRole('heading', { name: /Comments \(2\)/ })).toBeInTheDocument()
  })

  it('posts a merge with the chosen method and message', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.endsWith('/merge')) return { body: { pull_request: pull({ status: 'merged' }) } }
      return {
        body: {
          pull_request: pull(),
          comments: [],
          commits: [prCommit()],
          mergeable: { status: 'clean' },
        },
      }
    })
    renderAt('/pulls/1')
    await page.getByRole('button', { name: 'Merge' }).click()
    await page.getByRole('dialog').getByLabelText('Squash and merge').click()
    await page
      .getByRole('dialog')
      .getByLabelText(/Commit message/)
      .fill('squash it')
    await page.getByRole('button', { name: 'Merge pull request' }).click()
    await expect.poll(() => requests.find((r) => r.url.endsWith('/merge'))?.init?.body).toBeTruthy()
    const merge = requests.find((r) => r.url.endsWith('/merge'))
    expect(JSON.parse(String(merge?.init?.body))).toEqual({
      method: 'squash',
      message: 'squash it',
    })
  })

  it('closes an open pull request', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.endsWith('/status')) {
        return { body: { pull_request: pull({ status: 'closed' }) } }
      }
      return { body: { pull_request: pull(), comments: [], commits: [] } }
    })
    renderAt('/pulls/1')
    await page.getByRole('button', { name: 'Close' }).click()
    await expect.poll(() => requests.find((r) => r.url.endsWith('/status'))).toBeTruthy()
    const close = requests.find((r) => r.url.endsWith('/status'))
    expect(JSON.parse(String(close?.init?.body))).toEqual({ status: 'closed' })
  })
})

describe('route: /pulls/{id}/files', () => {
  it('renders the unified diff and posts a line-anchored comment', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.endsWith('/diff')) return { body: DIFF }
      if (url.endsWith('/comments') && init?.method === 'POST') {
        return { body: { comment: prComment({ id: 9 }) } }
      }
      return { body: { pull_request: pull(), comments: [], commits: [] } }
    })
    renderAt('/pulls/1/files')
    await expect.element(page.getByText('+added line')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Comment on a.txt line 2 (right side)' }).click()
    await page.getByLabelText('comment').fill('nice change')
    await page.getByRole('button', { name: 'Comment', exact: true }).click()
    await expect
      .poll(() => requests.find((r) => r.url.endsWith('/comments') && r.init?.method === 'POST'))
      .toBeTruthy()
    const post = requests.find((r) => r.url.endsWith('/comments') && r.init?.method === 'POST')
    expect(JSON.parse(String(post?.init?.body))).toEqual({
      body: 'nice change',
      path: 'a.txt',
      line: 2,
      side: 'right',
    })
  })

  it('pins the comment to the rendered head via the anchor header', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.endsWith('/diff')) return { body: DIFF, headers: { 'x-anchor-sha': 'abc123' } }
      if (url.endsWith('/comments') && init?.method === 'POST') {
        return { body: { comment: prComment({ id: 9 }) } }
      }
      return { body: { pull_request: pull(), comments: [], commits: [] } }
    })
    renderAt('/pulls/1/files')
    await page.getByRole('button', { name: 'Comment on a.txt line 2 (right side)' }).click()
    await page.getByLabelText('comment').fill('seen at abc123')
    await page.getByRole('button', { name: 'Comment', exact: true }).click()
    await expect
      .poll(() => requests.find((r) => r.url.endsWith('/comments') && r.init?.method === 'POST'))
      .toBeTruthy()
    const post = requests.find((r) => r.url.endsWith('/comments') && r.init?.method === 'POST')
    expect(JSON.parse(String(post?.init?.body))).toMatchObject({ anchor: 'abc123' })
  })

  it('warns when the diff response is truncated', async () => {
    stubAPI((url) => {
      if (url.endsWith('/diff')) return { body: DIFF, headers: { 'x-diff-truncated': 'true' } }
      return { body: { pull_request: pull(), comments: [], commits: [] } }
    })
    renderAt('/pulls/1/files')
    await expect.element(page.getByText(/truncated/)).toBeInTheDocument()
  })

  it('keeps the draft and shows the error when the comment post fails', async () => {
    stubAPI((url, init) => {
      if (url.endsWith('/diff')) return { body: DIFF, headers: { 'x-anchor-sha': 'abc123' } }
      if (url.endsWith('/comments') && init?.method === 'POST') {
        return {
          status: 422,
          body: {
            error: {
              message: 'Validation failed.',
              fields: { anchor: 'Anchor must be a commit on the head branch.' },
            },
          },
        }
      }
      return { body: { pull_request: pull(), comments: [], commits: [] } }
    })
    renderAt('/pulls/1/files')
    await page.getByRole('button', { name: 'Comment on a.txt line 2 (right side)' }).click()
    await page.getByLabelText('comment').fill('draft survives')
    await page.getByRole('button', { name: 'Comment', exact: true }).click()
    // The field error's reason, not the generic "Validation failed.".
    await expect
      .element(page.getByText('Anchor must be a commit on the head branch.'))
      .toBeInTheDocument()
    expect(page.getByLabelText('comment')).toHaveValue('draft survives')
  })

  it('ignores a second Ctrl+Enter while the first comment is in flight', async () => {
    let posts = 0
    let release: (() => void) | undefined
    const gate = new Promise<void>((r) => {
      release = r
    })
    stubAPI((url, init) => {
      if (url.endsWith('/diff')) return { body: DIFF }
      if (url.endsWith('/comments') && init?.method === 'POST') {
        posts++
        return gate.then(() => ({ body: { comment: prComment({ id: 9 }) } }))
      }
      return { body: { pull_request: pull(), comments: [], commits: [] } }
    })
    renderAt('/pulls/1/files')
    await page.getByRole('button', { name: 'Comment on a.txt line 2 (right side)' }).click()
    const ta = page.getByLabelText('comment')
    await ta.fill('one shot')
    // The disabled Comment button does not stop requestSubmit: two real
    // keydowns while the first POST is still pending must post once.
    const ctrlEnter = () =>
      ta
        .element()
        .dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
    ctrlEnter()
    ctrlEnter()
    await expect.poll(() => posts).toBe(1)
    release?.()
    await expect.poll(() => page.getByLabelText('comment').elements().length).toBe(0)
    expect(posts).toBe(1)
  })

  it('anchors deleted lines to the old path of a whitespace rename', async () => {
    // Renaming "a.txt " to "a.txt": the old operand ends in a space plus
    // git's tab marker; left-side comments must keep the real old name.
    const RENAME_DIFF = [
      'diff --git a/a.txt  b/a.txt',
      'similarity index 50%',
      'rename from a.txt ',
      'rename to a.txt',
      'index 111..222 100644',
      '--- a/a.txt \t',
      '+++ b/a.txt',
      '@@ -1,2 +1,2 @@',
      ' one',
      '-old',
      '+new',
    ].join('\n')
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.endsWith('/diff')) return { body: RENAME_DIFF }
      if (url.endsWith('/comments') && init?.method === 'POST') {
        return { body: { comment: prComment({ id: 9 }) } }
      }
      return { body: { pull_request: pull(), comments: [], commits: [] } }
    })
    renderAt('/pulls/1/files')
    await page.getByRole('button', { name: 'Comment on a.txt  line 2 (left side)' }).click()
    await page.getByLabelText('comment').fill('was the old name')
    await page.getByRole('button', { name: 'Comment', exact: true }).click()
    await expect
      .poll(() => requests.find((r) => r.url.endsWith('/comments') && r.init?.method === 'POST'))
      .toBeTruthy()
    const post = requests.find((r) => r.url.endsWith('/comments') && r.init?.method === 'POST')
    expect(JSON.parse(String(post?.init?.body))).toMatchObject({
      path: 'a.txt ',
      side: 'left',
    })
  })
})

describe('nav: pulls availability', () => {
  it('shows the Pulls link when the API answers', async () => {
    stubAPI((url) => {
      if (url.includes('/pulls')) return { body: { pulls: [], total: 0, page: 1, per_page: 20 } }
      return { body: { workspaces: [] } }
    })
    renderAt('/')
    await expect.element(page.getByRole('link', { name: 'Pulls' }).first()).toBeInTheDocument()
  })

  it('hides the Pulls link when the API 404s', async () => {
    stubAPI((url) => {
      if (url.includes('/pulls')) return { status: 404, body: {} }
      return { body: { workspaces: [] } }
    })
    renderAt('/')
    await expect.element(page.getByRole('link', { name: 'Issues' })).toBeInTheDocument()
    expect(page.getByRole('link', { name: 'Pulls' }).elements()).toHaveLength(0)
  })
})
