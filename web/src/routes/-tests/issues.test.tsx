import { page } from 'vitest/browser'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Comment, Issue, Label, LabelInput, Workspace } from '../../api'
import { renderAt, stubAPI } from '../../test-utils'

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
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

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

describe('route: /issues', () => {
  it('shows issues fetched through the loader', async () => {
    stubAPI((url) => {
      if (url.includes('/labels'))
        return { body: { labels: [label({ name: 'Bug' }), label({ id: 2, name: 'Feature' })] } }
      if (url.includes('/pulls')) return { status: 404, body: {} } // availability probe
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
    renderAt('/w/default/issues')
    await expect.element(page.getByText(/from api/)).toBeInTheDocument()
    await expect.element(page.getByText(/second/)).toBeInTheDocument()
    await expect.element(page.getByText('Showing 1–2 of 2 issues')).toBeInTheDocument()
  })

  it('passes search params to the API', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      if (url.includes('/labels')) return { body: { labels: [] } }
      if (url.includes('/pulls')) return { status: 404, body: {} } // availability probe
      calls.push(url)
      return { body: { issues: [], total: 0, page: 1, per_page: 20 } }
    })
    renderAt('/w/default/issues?status=closed&q=bug')
    await expect.element(page.getByText('No issues match the current filters.')).toBeInTheDocument()
    expect(calls[0]).toContain('status=closed')
    expect(calls[0]).toContain('q=bug')
  })

  it('puts label/sort/page filters in the URL and sends them to the API', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      if (url.includes('/labels'))
        return { body: { labels: [label({ name: 'Bug' }), label({ id: 2, name: 'Feature' })] } }
      if (url.includes('/pulls')) return { status: 404, body: {} } // availability probe
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
    renderAt('/w/default/issues?label=Bug&sort=oldest&page=2')
    await expect.element(page.getByText(/filtered/)).toBeInTheDocument()
    expect(calls[0]).toContain('label=Bug')
    expect(calls[0]).toContain('sort=oldest')
    expect(calls[0]).toContain('page=2')
    // label select is populated from the labels endpoint
    await expect.element(page.getByRole('option', { name: 'Feature' })).toBeInTheDocument()
    // page 2 of 2 shows a Previous link back to page 1
    await expect.element(page.getByRole('link', { name: 'Previous' })).toBeInTheDocument()
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
    renderAt('/w/default/issues')
    await page.getByRole('link', { name: 'Next' }).click()
    // the page=2 request proves the link drove a URL-driven loader refetch
    await expect.poll(() => calls.some((u) => u.includes('page=2'))).toBe(true)
    await expect.element(page.getByText('Page 1 of 2')).toBeInTheDocument()
  })
})

describe('route: /issues/$issueId/edit', () => {
  it('loads current values and PATCHes title/body/label_ids', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.includes('/labels'))
        return { body: { labels: [label({ name: 'Bug' }), label({ id: 2, name: 'Feature' })] } }
      if (init?.method === 'PATCH') return { body: { issue: issue({ title: 'renamed' }) } }
      return { body: { issue: issue(), comments: [] } }
    })
    renderAt('/w/default/issues/1/edit')
    const title = page.getByLabelText(/Title/)
    await expect.element(title).toHaveValue('first issue')
    await title.fill('renamed')
    await page.getByRole('checkbox', { name: 'Feature' }).click()
    await page.getByRole('button', { name: 'Save changes' }).click()

    // navigated back to the detail page — proves the PATCH ran first
    await expect.element(page.getByRole('heading', { name: /#1/ })).toBeInTheDocument()
    const patch = requests.find((r) => r.init?.method === 'PATCH')
    expect(patch?.url).toContain('/api/v1/w/default/issues/1')
    expect(JSON.parse(String(patch?.init?.body))).toEqual({
      title: 'renamed',
      body: 'body text',
      label_ids: [2],
    })
  })
})

describe('route: /issues/$issueId', () => {
  it('shows issue and comments, then posts a status change', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
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
    renderAt('/w/default/issues/1')
    // the delete dialog's description also contains the title — match the
    // heading only
    await expect.element(page.getByRole('heading', { name: /detail page/ })).toBeInTheDocument()
    await expect.element(page.getByText('nice work')).toBeInTheDocument()

    await page.getByRole('button', { name: 'Close issue' }).click()
    await expect.element(page.getByRole('button', { name: 'Reopen issue' })).toBeInTheDocument()
    const statusCall = requests.find((r) => r.url.endsWith('/status'))
    expect(statusCall?.init?.method).toBe('POST')
    expect(JSON.parse(String(statusCall?.init?.body))).toEqual({ status: 'closed' })
  })

  it('each comment has a #comment-N anchor and a permalink button', async () => {
    stubAPI(() => ({ body: { issue: issue(), comments: [comment()] } }))
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    expect(document.getElementById('comment-1')).not.toBeNull()
    await expect.element(page.getByRole('button', { name: 'copy link' })).toBeInTheDocument()
  })

  it('edits a comment inline and shows the edited badge', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    let edited = false
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (init?.method === 'PATCH' && url.includes('/comments/')) {
        edited = true
        return {
          body: { comment: comment({ body: 'edited body', edited_at: '2026-01-02T00:00:00Z' }) },
        }
      }
      return {
        body: {
          issue: issue(),
          comments: [
            comment(edited ? { body: 'edited body', edited_at: '2026-01-02T00:00:00Z' } : {}),
          ],
        },
      }
    })
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Edit comment 1' }).click()
    await page.getByLabelText('comment body').fill('edited body')
    await page.getByRole('button', { name: 'Save changes' }).click()

    await expect.element(page.getByText('edited', { exact: true })).toBeInTheDocument()
    const patch = requests.find((r) => r.init?.method === 'PATCH')
    expect(patch?.url).toContain('/comments/1')
    expect(JSON.parse(String(patch?.init?.body))).toEqual({ body: 'edited body' })
  })

  it('disables the comment edit textarea while saving is in flight', async () => {
    let finishPatch: (() => void) | undefined
    let edited = false
    stubAPI((url, init) => {
      if (init?.method === 'PATCH' && url.includes('/comments/')) {
        return new Promise((resolve) => {
          finishPatch = () => {
            edited = true
            resolve({
              body: {
                comment: comment({ body: 'edited body', edited_at: '2026-01-02T00:00:00Z' }),
              },
            })
          }
        })
      }
      return {
        body: {
          issue: issue(),
          comments: [
            comment(edited ? { body: 'edited body', edited_at: '2026-01-02T00:00:00Z' } : {}),
          ],
        },
      }
    })
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Edit comment 1' }).click()
    await page.getByLabelText('comment body').fill('edited body')
    await page.getByRole('button', { name: 'Save changes' }).click()

    await expect.element(page.getByLabelText('comment body')).toBeDisabled()
    finishPatch?.()
    await expect.element(page.getByText('edited', { exact: true })).toBeInTheDocument()
  })

  it('keeps a comment edit open and announces the error when saving is rejected', async () => {
    stubAPI((url, init) => {
      if (init?.method === 'PATCH' && url.includes('/comments/')) {
        return {
          status: 422,
          body: {
            error: {
              message: 'Validation failed.',
              fields: { body: 'Body must be at most 10,000 characters.' },
            },
          },
        }
      }
      return { body: { issue: issue(), comments: [comment()] } }
    })
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Edit comment 1' }).click()
    await page.getByLabelText('comment body').fill('edited body')
    await page.getByRole('button', { name: 'Save changes' }).click()

    await expect
      .element(page.getByRole('alert'))
      .toHaveTextContent('Body must be at most 10,000 characters.')
    await expect.element(page.getByLabelText('comment body')).toHaveValue('edited body')
    const textarea = document.getElementById('comment-edit-1')
    expect(textarea?.getAttribute('data-error')).toBe('comment-edit-1-error')
    expect(textarea?.getAttribute('aria-invalid')).toBe('true')
    expect(textarea?.getAttribute('aria-describedby')).toBe('comment-edit-1-api-error')

    await page.getByRole('button', { name: 'Cancel' }).click()
    await page.getByRole('button', { name: 'Edit comment 1' }).click()
    const reopened = document.getElementById('comment-edit-1')
    expect(reopened?.getAttribute('aria-invalid')).toBe('false')
    expect(reopened?.getAttribute('aria-describedby')).toBeNull()
  })

  it('links the required-field error after blurring an empty comment edit', async () => {
    stubAPI(() => ({ body: { issue: issue(), comments: [comment()] } }))
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Edit comment 1' }).click()
    await page.getByLabelText('comment body').fill('')
    const textarea = document.getElementById('comment-edit-1') as HTMLTextAreaElement
    textarea.blur()

    await expect.poll(() => textarea.getAttribute('aria-invalid')).toBe('true')
    expect(textarea.getAttribute('aria-describedby')).toContain('comment-edit-1-error')
    await expect
      .poll(() => document.querySelector('div.visually-hidden')?.textContent)
      .toContain('Body is required.')
  })

  it('does not announce a required-field error after cancel removes the editor', async () => {
    stubAPI(() => ({ body: { issue: issue(), comments: [comment()] } }))
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    await page.getByRole('button', { name: 'Edit comment 1' }).click()
    await page.getByLabelText('comment body').fill('')
    const textarea = document.getElementById('comment-edit-1') as HTMLTextAreaElement
    textarea.blur()
    const cancel = textarea
      .closest('form')
      ?.querySelector<HTMLButtonElement>('button[type="button"]')
    expect(cancel?.textContent).toBe('Cancel')
    cancel?.click()

    await new Promise((resolve) => setTimeout(resolve, 600))
    expect(document.querySelector('div.visually-hidden')?.textContent).toBe('')
  })

  it('deletes a comment through the confirm dialog', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      return { body: { issue: issue(), comments: [comment()] } }
    })
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    // the comment's delete trigger lives inside its <li> (the issue's
    // own delete button is in the header)
    await page.getByRole('listitem').getByRole('button', { name: 'Delete' }).click()
    await page.getByRole('dialog').getByRole('button', { name: 'Delete comment' }).click()
    await expect.poll(() => requests.find((r) => r.init?.method === 'DELETE')).toBeTruthy()
    expect(requests.find((r) => r.init?.method === 'DELETE')?.url).toContain('/comments/1')
  })

  it('disables the comment submit while the body is blank', async () => {
    stubAPI(() => ({ body: { issue: issue(), comments: [] } }))
    renderAt('/w/default/issues/1')
    const submit = page.getByRole('button', { name: 'Comment', exact: true })
    await expect.element(submit).toBeDisabled()
    await page.getByLabelText(/Add a comment/).fill('a draft')
    await expect.element(submit).toBeEnabled()
    await page.getByLabelText(/Add a comment/).fill('   ')
    await expect.element(submit).toBeDisabled()
  })

  it('resets browser validation state after posting a comment', async () => {
    let posted = false
    stubAPI((url, init) => {
      if (init?.method === 'POST' && url.endsWith('/comments')) {
        posted = true
        return { status: 201, body: { comment: comment({ id: 2, body: 'posted body' }) } }
      }
      return {
        body: {
          issue: issue(),
          comments: posted ? [comment({ id: 2, body: 'posted body' })] : [],
        },
      }
    })
    renderAt('/w/default/issues/1')
    await page.getByLabelText(/Add a comment/).fill('posted body')
    await page.getByRole('button', { name: 'Comment', exact: true }).click()
    await expect.element(page.getByLabelText(/Add a comment/)).toHaveValue('')
    await expect
      .poll(() => document.getElementById('comment-body')?.matches(':user-invalid'))
      .toBe(false)
  })

  it('disables the comment textarea while the post is in flight', async () => {
    let finishPost: (() => void) | undefined
    stubAPI((url, init) => {
      if (init?.method === 'POST' && url.endsWith('/comments')) {
        return new Promise((resolve) => {
          finishPost = () =>
            resolve({ status: 201, body: { comment: comment({ id: 2, body: 'posted body' }) } })
        })
      }
      return { body: { issue: issue(), comments: [] } }
    })
    renderAt('/w/default/issues/1')
    await page.getByLabelText(/Add a comment/).fill('posted body')
    await page.getByRole('button', { name: 'Comment', exact: true }).click()
    await expect.element(page.getByLabelText(/Add a comment/)).toBeDisabled()
    finishPost?.()
    await expect.element(page.getByLabelText(/Add a comment/)).toHaveValue('')
  })

  it('keeps the draft and shows the field error when a comment is rejected', async () => {
    stubAPI((url, init) => {
      if (init?.method === 'POST' && url.endsWith('/comments')) {
        return {
          status: 422,
          body: {
            error: {
              message: 'Validation failed.',
              fields: { body: 'Body must be at most 10,000 characters.' },
            },
          },
        }
      }
      return { body: { issue: issue(), comments: [] } }
    })
    renderAt('/w/default/issues/1')
    await page.getByLabelText(/Add a comment/).fill('too long')
    await page.getByRole('button', { name: 'Comment', exact: true }).click()
    await expect.element(page.getByLabelText(/Add a comment/)).toHaveValue('too long')
    await expect
      .element(page.getByText('Body must be at most 10,000 characters.'))
      .toBeInTheDocument()
    const textarea = document.getElementById('comment-body')
    expect(textarea?.getAttribute('aria-invalid')).toBe('true')
    expect(textarea?.getAttribute('aria-describedby')).toContain('comment-body-api-error')
  })

  it('confirms before leaving with a comment draft, and navigates on accept', async () => {
    stubAPI((url) => {
      if (url.includes('/labels')) return { body: { labels: [] } }
      if (url.includes('/pulls')) return { status: 404, body: {} }
      if (url.endsWith('/issues') || url.includes('/issues?')) {
        return { body: { issues: [], total: 0, page: 1, per_page: 20 } }
      }
      return { body: { issue: issue(), comments: [] } }
    })
    renderAt('/w/default/issues/1')
    await page.getByLabelText(/Add a comment/).fill('draft to lose')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const breadcrumb = page.getByRole('navigation', { name: /breadcrumb/i })
    await breadcrumb.getByRole('link', { name: 'Issues' }).click()
    await expect.poll(() => confirm.mock.calls.length > 0).toBeTruthy()
    // declined: still on the detail page, draft intact
    await expect.element(page.getByLabelText(/Add a comment/)).toHaveValue('draft to lose')
    confirm.mockReturnValue(true)
    await breadcrumb.getByRole('link', { name: 'Issues' }).click()
    await expect.element(page.getByText('No issues here yet.')).toBeInTheDocument()
    confirm.mockRestore()
  })

  it('asks once for several dirty drafts and re-arms on the next page', async () => {
    stubAPI((url) => {
      if (url.includes('/labels')) return { body: { labels: [] } }
      if (url.includes('/pulls')) return { status: 404, body: {} }
      if (url.endsWith('/issues') || url.includes('/issues?')) {
        return { body: { issues: [], total: 0, page: 1, per_page: 20 } }
      }
      return { body: { issue: issue(), comments: [comment()] } }
    })
    const { router } = renderAt('/w/default/issues/1')
    // two independent dirty guards: the new-comment box and a comment edit
    await page.getByLabelText(/Add a comment/).fill('draft one')
    await page.getByRole('button', { name: 'Edit comment 1' }).click()
    await page.getByLabelText('comment body').fill('edited draft')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const breadcrumb = page.getByRole('navigation', { name: /breadcrumb/i })
    await breadcrumb.getByRole('link', { name: 'Issues' }).click()
    await expect.element(page.getByText('No issues here yet.')).toBeInTheDocument()
    // one accepted confirm releases every guard on the page
    expect(confirm).toHaveBeenCalledTimes(1)
    // the latch re-arms on navigation: a fresh draft on the detail page
    // asks again (renderAt wires the same onResolved reset as main.tsx)
    await router.navigate({
      to: '/w/$ws/issues/$issueId',
      params: { ws: 'default', issueId: '1' },
    })
    await page.getByLabelText(/Add a comment/).fill('draft two')
    await breadcrumb.getByRole('link', { name: 'Issues' }).click()
    await expect.element(page.getByText('No issues here yet.')).toBeInTheDocument()
    expect(confirm).toHaveBeenCalledTimes(2)
    confirm.mockRestore()
  })

  it('hides a comment with a reason and unhides it', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    let hidden = false
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (url.endsWith('/hide')) {
        hidden = true
        return { body: { comment: comment({ hidden: true, hidden_reason: 'spam' }) } }
      }
      if (url.endsWith('/unhide')) {
        hidden = false
        return { body: { comment: comment() } }
      }
      return {
        body: {
          issue: issue(),
          comments: [comment(hidden ? { hidden: true, hidden_reason: 'spam' } : {})],
        },
      }
    })
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('a comment')).toBeInTheDocument()

    // hide via the reason dialog
    await page.getByRole('button', { name: 'Hide comment 1' }).click()
    await page.getByLabelText(/Reason/).selectOptions('spam')
    await page.getByRole('dialog').getByRole('button', { name: 'Hide comment' }).click()
    await expect.element(page.getByText('This comment was marked as spam.')).toBeInTheDocument()
    const hide = requests.find((r) => r.url.endsWith('/hide'))
    expect(JSON.parse(String(hide?.init?.body))).toEqual({ reason: 'spam' })

    // unhide restores the body
    await page.getByRole('button', { name: 'Unhide comment 1' }).click()
    await expect.element(page.getByText('a comment')).toBeInTheDocument()
    expect(requests.some((r) => r.url.endsWith('/unhide'))).toBe(true)
  })

  it('renders bodies as markdown and neutralizes unsafe markup', async () => {
    stubAPI(() => ({
      body: {
        issue: issue({
          body: '**bold**\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))\n\n> [!NOTE]\n> look out',
        }),
        comments: [comment({ body: '- [x] done' })],
      },
    }))
    renderAt('/w/default/issues/1')
    await expect.element(page.getByText('bold')).toBeInTheDocument()
    expect(document.querySelector('article strong')?.textContent).toBe('bold')
    // raw HTML is escaped, unsafe hrefs stripped; task list rendered
    expect(document.querySelector('article script')).toBeNull()
    expect(document.querySelector('a[href^="javascript:"]')).toBeNull()
    expect(document.querySelector('input[type=checkbox]')).not.toBeNull()
    // GitHub-style alert renders via the remark plugin
    expect(document.querySelector('article .markdown-alert-note')).not.toBeNull()
  })

  it('copies the issue as markdown via the export endpoint', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      calls.push(url)
      if (url.endsWith('/markdown')) return { body: '# Issue\n\nbody text' }
      return { body: { issue: issue(), comments: [] } }
    })
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    renderAt('/w/default/issues/1')
    await page.getByRole('button', { name: 'Copy as markdown' }).click()
    await expect.element(page.getByText('copied')).toBeInTheDocument()
    expect(calls.some((u) => u.endsWith('/issues/1/markdown'))).toBe(true)
    expect(writeText).toHaveBeenCalled()
  })
})

describe('route: /labels', () => {
  it('lists labels by kind plus archived, and creates/archives/restores', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    const active: Label[] = [
      label({ name: 'Bug' }),
      label({ id: 2, name: 'q', kind: 'discussion' }),
    ]
    const archived: Label[] = []
    let nextId = 9
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (!url.includes('/labels')) return { status: 404, body: {} }
      if (url.includes('archived=1')) return { body: { labels: archived } }
      if (init?.method === 'POST') {
        const b = JSON.parse(String(init.body)) as LabelInput
        const created = label({ id: nextId++, ...b, workspace_id: b.shared ? null : 1 })
        const i = archived.findIndex((l) => l.name === b.name)
        if (i >= 0) archived.splice(i, 1)
        active.push(created)
        return { status: 201, body: { label: created } }
      }
      if (init?.method === 'DELETE') {
        const i = active.findIndex((l) => l.id === 1)
        const [removed] = active.splice(i, 1)
        if (removed) archived.push({ ...removed, archived: true })
        return { status: 204 }
      }
      return { body: { labels: active } }
    })
    renderAt('/w/default/labels')
    // grouped by kind
    await expect.element(page.getByText('Issue labels')).toBeInTheDocument()
    await expect.element(page.getByText('Bug')).toBeInTheDocument()
    await expect.element(page.getByText('Discussion labels')).toBeInTheDocument()
    await expect.element(page.getByText('q', { exact: true })).toBeInTheDocument()

    // create sends name/color/kind/scope
    await page.getByLabelText('Name').fill('triage')
    await page.getByRole('button', { name: 'Create label' }).click()
    await expect.element(page.getByText('triage')).toBeInTheDocument()
    const post = requests.find((r) => r.init?.method === 'POST')
    expect(JSON.parse(String(post?.init?.body))).toMatchObject({
      name: 'triage',
      kind: 'issue',
      shared: false,
    })

    // archive via the confirm dialog moves it to the Archived section
    // (the row button says "Delete" like the MPA; it archives, not destroys)
    await page
      .getByRole('listitem')
      .filter({ hasText: 'Bug' })
      .getByRole('button', { name: 'Delete' })
      .click()
    await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()
    await expect.element(page.getByRole('button', { name: 'Restore' })).toBeInTheDocument()
    expect(requests.find((r) => r.init?.method === 'DELETE')?.url).toContain('/labels/1')

    // restore re-POSTs the same name/scope and it returns to active
    await page.getByRole('button', { name: 'Restore' }).click()
    await expect.element(page.getByRole('button', { name: 'Restore' })).not.toBeInTheDocument()
    await expect.element(page.getByText('Bug')).toBeInTheDocument()
    expect(requests.filter((r) => r.init?.method === 'POST')).toHaveLength(2)
  })
})

describe('workspace routing', () => {
  it('redirects workspace-less URLs to the default workspace', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      calls.push(url)
      if (url.endsWith('/api/v1/workspaces')) return { body: { workspaces: [workspace()] } }
      if (url.includes('/labels')) return { body: { labels: [] } }
      return { body: { issues: [issue({ title: 'landed' })], total: 1, page: 1, per_page: 20 } }
    })
    renderAt('/issues?q=hi')
    await expect.element(page.getByText(/landed/)).toBeInTheDocument()
    // resolved to /w/default/issues and the search params survived
    const listCall = calls.find((u) => u.includes('/w/default/issues'))
    expect(listCall).toContain('q=hi')
  })

  it('rebases a cross-workspace GET onto the owning workspace', async () => {
    const calls: string[] = []
    stubAPI((url) => {
      calls.push(url)
      if (url.includes('/w/other/')) {
        return { status: 404, body: { error: { message: 'Not found.', workspace: 'default' } } }
      }
      return { body: { issue: issue({ title: 'real home' }), comments: [] } }
    })
    renderAt('/w/other/issues/1')
    await expect.element(page.getByRole('heading', { name: /real home/ })).toBeInTheDocument()
    expect(calls.some((u) => u.includes('/w/default/issues/1'))).toBe(true)
  })

  it('keeps a #comment-N permalink through both redirects', async () => {
    stubAPI((url) => {
      if (url.endsWith('/api/v1/workspaces')) return { body: { workspaces: [workspace()] } }
      if (url.includes('/w/other/')) {
        return { status: 404, body: { error: { message: 'Not found.', workspace: 'default' } } }
      }
      return { body: { issue: issue(), comments: [comment({ id: 2 })] } }
    })
    const { router } = renderAt('/issues/1#comment-2')
    await expect.element(page.getByRole('heading', { name: /first issue/ })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/w/default/issues/1')
    expect(router.state.location.hash).toBe('comment-2')
  })

  it('keeps a #comment-N permalink across a cross-workspace rebase', async () => {
    stubAPI((url) => {
      if (url.includes('/w/other/')) {
        return { status: 404, body: { error: { message: 'Not found.', workspace: 'default' } } }
      }
      return { body: { issue: issue(), comments: [comment({ id: 2 })] } }
    })
    const { router } = renderAt('/w/other/issues/1#comment-2')
    await expect.element(page.getByRole('heading', { name: /first issue/ })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/w/default/issues/1')
    expect(router.state.location.hash).toBe('comment-2')
  })

  it('shows the not-found page for an unknown workspace slug', async () => {
    stubAPI(() => ({ status: 404, body: { error: { message: 'Not found.' } } }))
    renderAt('/w/nope/issues')
    await expect.element(page.getByText('404 — not found')).toBeInTheDocument()
  })
})

describe('route: /workspaces', () => {
  it('lists workspaces and deletes one through the confirm dialog', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    const wss = [
      workspace(),
      workspace({ id: 2, slug: 'sandbox', name: 'Sandbox', issue_count: 3 }),
    ]
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (init?.method === 'DELETE') {
        wss.splice(1, 1)
        return { status: 204 }
      }
      return { body: { workspaces: wss } }
    })
    renderAt('/workspaces')
    await expect.element(page.getByText('w/default')).toBeInTheDocument()
    await expect.element(page.getByText('Sandbox')).toBeInTheDocument()
    await expect.element(page.getByText('3 issues')).toBeInTheDocument()

    await page
      .getByRole('listitem')
      .filter({ hasText: 'w/sandbox' })
      .getByRole('button', { name: 'Delete' })
      .click()
    await page.getByRole('dialog').getByRole('button', { name: 'Delete workspace' }).click()
    await expect
      .poll(() => requests.find((r) => r.init?.method === 'DELETE')?.url)
      .toContain('/api/v1/workspaces/sandbox')
    await expect.element(page.getByText('w/sandbox')).not.toBeInTheDocument()
  })

  it('disables deletion of the last remaining workspace', async () => {
    stubAPI(() => ({ body: { workspaces: [workspace()] } }))
    renderAt('/workspaces')
    await expect
      .element(page.getByText('The last remaining workspace cannot be deleted.'))
      .toBeInTheDocument()
    expect(page.getByRole('button', { name: 'Delete' }).query()).toBeNull()
  })

  it('creates a workspace and enters it', async () => {
    const requests: { url: string; init?: RequestInit | undefined }[] = []
    stubAPI((url, init) => {
      requests.push({ url, init })
      if (init?.method === 'POST' && url.endsWith('/api/v1/workspaces')) {
        return { status: 201, body: { workspace: workspace({ id: 2, slug: 'newws' }) } }
      }
      if (url.includes('/labels')) return { body: { labels: [] } }
      return { body: { issues: [], total: 0, page: 1, per_page: 20 } }
    })
    renderAt('/workspaces/new')
    await page.getByLabelText(/Slug/).fill('newws')
    await page.getByRole('button', { name: 'Create workspace' }).click()
    // lands inside the new workspace's issue list
    await expect.poll(() => requests.some((r) => r.url.includes('/w/newws/issues'))).toBe(true)
    const post = requests.find((r) => r.init?.method === 'POST')
    expect(JSON.parse(String(post?.init?.body))).toEqual({
      slug: 'newws',
      name: '',
      description: '',
    })
  })
})

describe('404 handling', () => {
  it('shows the not-found page for a missing issue', async () => {
    stubAPI(() => ({ status: 404, body: { error: { message: 'Not found.' } } }))
    renderAt('/w/default/issues/999')
    await expect.element(page.getByText('404 — not found')).toBeInTheDocument()
  })

  it('shows the not-found page for an unmatched route', async () => {
    stubAPI(() => ({ status: 404, body: { error: { message: 'Not found.' } } }))
    renderAt('/no-such-page')
    await expect.element(page.getByText('404 — not found')).toBeInTheDocument()
  })
})
