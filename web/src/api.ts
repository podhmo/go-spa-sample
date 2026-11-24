// Typed client for the ported zatudan JSON API (internal/api).
// In dev, requests go through the vite proxy which injects the
// Bearer credential — see vite.config.ts.

export type User = { id: number; name: string }

export type Label = {
  id: number
  name: string
  color: string
  kind: string
  workspace_id: number | null
  archived: boolean
}

export type Issue = {
  id: number
  title: string
  body: string
  status: 'open' | 'closed'
  labels: Label[]
  author: User | null
  created_at: string
  updated_at: string
}

export type Moderation = {
  edited_at?: string
  hidden: boolean
  hidden_reason?: string
}

export type Comment = {
  id: number
  body: string
  author: User | null
  created_at: string
} & Moderation

export type IssueList = {
  issues: Issue[]
  total: number
  page: number
  per_page: number
}

export type IssueDetail = { issue: Issue; comments: Comment[] }

export type IssueInput = { title: string; body: string; label_ids?: number[] }

export type LabelInput = { name: string; color: string; kind: string; shared: boolean }

export type DiscussionCategory = {
  id: number
  name: string
  slug: string
  description: string
  format: 'open' | 'qa'
}

export type Discussion = {
  id: number
  title: string
  body: string
  category: DiscussionCategory
  labels: Label[]
  author: User | null
  answered: boolean
  created_at: string
  updated_at: string
}

// A reply sits one level under a thread (the MPA's "comments on a
// comment"); threads and replies share the moderation verbs.
export type DiscussionReply = {
  id: number
  thread_id: number
  body: string
  author: User | null
  created_at: string
} & Moderation

export type DiscussionThread = {
  id: number
  is_answer: boolean
  body: string
  author: User | null
  created_at: string
  comments: DiscussionReply[]
} & Moderation

export type DiscussionList = {
  discussions: Discussion[]
  total: number
  page: number
  per_page: number
}

export type DiscussionDetail = { discussion: Discussion; threads: DiscussionThread[] }

// category_id wins when both are given; category names the slug for
// convenience (the API resolves it to the id).
export type DiscussionInput = {
  title: string
  body: string
  category_id?: number | undefined
  category?: string | undefined
  label_ids?: number[] | undefined
}

export type DiscussionListFilter = {
  category?: string | undefined
  answered?: string | undefined
  q?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

export type PullRequest = {
  id: number
  title: string
  body: string
  status: 'open' | 'closed' | 'merged'
  base: string
  head: string
  author: User | null
  head_sha?: string
  merge_sha?: string
  merge_method?: string
  created_at: string
  updated_at: string
  closed_at?: string
}

export type PRComment = {
  id: number
  body: string
  path?: string
  line?: number
  side?: string
  anchor_sha?: string
  diff_snippet?: string
  author: User | null
  created_at: string
} & Moderation

export type PRCommit = { sha: string; subject: string; author: string; date: string }

export type PullRef = { name: string; short: string }

export type PullMergeable =
  | { status: 'clean' | 'conflicting' | 'unknown'; files?: string[] }
  | undefined

export type PullList = { pulls: PullRequest[]; total: number; page: number; per_page: number }

export type PullDetail = {
  pull_request: PullRequest
  comments: PRComment[]
  commits: PRCommit[]
  mergeable?: PullMergeable
}

export type PullListFilter = {
  status?: string | undefined
  q?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

export type PullInput = { title: string; body: string; base: string; head: string }

export type PRCommentInput = {
  body: string
  path?: string
  line?: number
  side?: string
  // The head revision the commenter was looking at (X-Anchor-SHA from
  // the diff endpoint) — keeps the comment pinned to the rendered code
  // even if the head branch moved in the meantime.
  anchor?: string | undefined
}

export type PullDiff = {
  text: string
  head_sha?: string | undefined
  truncated: boolean
}

export type Workspace = {
  id: number
  slug: string
  name: string
  description: string
  issue_count: number
  discussion_count: number
  created_at: string
}

export type WorkspaceInput = { slug: string; name?: string; description?: string }

export type ApiError = {
  error: {
    message: string
    fields?: Record<string, string>
    // On a cross-scope GET 404 the API names the workspace that actually
    // owns the resource, so the client can rebase onto /w/{slug}/...
    // itself (the in-band counterpart of the MPA's 302).
    workspace?: string
  }
}

export class RequestError extends Error {
  status: number
  apiError: ApiError['error'] | null
  constructor(status: number, apiError: ApiError['error'] | null) {
    super(apiError?.message ?? `HTTP ${status}`)
    this.status = status
    this.apiError = apiError
  }
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    let apiError: ApiError['error'] | null = null
    try {
      apiError = (await res.json()).error ?? null
    } catch {
      // non-JSON error body
    }
    throw new RequestError(res.status, apiError)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

// fetchText is the non-JSON counterpart of fetchJSON, for the text/markdown
// export endpoints.
async function fetchText(path: string): Promise<string> {
  const res = await fetch(path)
  if (!res.ok) {
    let apiError: ApiError['error'] | null = null
    try {
      apiError = (await res.json()).error ?? null
    } catch {
      // non-JSON error body
    }
    throw new RequestError(res.status, apiError)
  }
  return res.text()
}

function qs(params: Record<string, string | number | undefined>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

const json = (body: unknown): RequestInit => ({
  headers: { 'content-type': 'application/json' },
  body: JSON.stringify(body),
})

// exactOptionalPropertyTypes: every field may be present-but-undefined
// (validateSearch fills all keys; qs() drops the undefined ones).
export type IssueListFilter = {
  status?: string | undefined
  q?: string | undefined
  label?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

export const api = {
  listIssues: (ws: string, f: IssueListFilter) =>
    fetchJSON<IssueList>(
      `/api/v1/w/${ws}/issues${qs({ status: f.status, q: f.q, label: f.label, sort: f.sort, page: f.page })}`,
    ),
  getIssue: (ws: string, id: number) => fetchJSON<IssueDetail>(`/api/v1/w/${ws}/issues/${id}`),
  createIssue: (ws: string, input: IssueInput) =>
    fetchJSON<{ issue: Issue }>(`/api/v1/w/${ws}/issues`, { method: 'POST', ...json(input) }),
  updateIssue: (ws: string, id: number, input: Partial<IssueInput>) =>
    fetchJSON<{ issue: Issue }>(`/api/v1/w/${ws}/issues/${id}`, {
      method: 'PATCH',
      ...json(input),
    }),
  setIssueStatus: (ws: string, id: number, status: 'open' | 'closed') =>
    fetchJSON<{ issue: Issue }>(`/api/v1/w/${ws}/issues/${id}/status`, {
      method: 'POST',
      ...json({ status }),
    }),
  deleteIssue: (ws: string, id: number) =>
    fetchJSON<void>(`/api/v1/w/${ws}/issues/${id}`, { method: 'DELETE' }),
  createComment: (ws: string, id: number, body: string) =>
    fetchJSON<{ comment: Comment }>(`/api/v1/w/${ws}/issues/${id}/comments`, {
      method: 'POST',
      ...json({ body }),
    }),
  updateComment: (ws: string, id: number, cid: number, body: string) =>
    fetchJSON<{ comment: Comment }>(`/api/v1/w/${ws}/issues/${id}/comments/${cid}`, {
      method: 'PATCH',
      ...json({ body }),
    }),
  deleteComment: (ws: string, id: number, cid: number) =>
    fetchJSON<void>(`/api/v1/w/${ws}/issues/${id}/comments/${cid}`, { method: 'DELETE' }),
  hideComment: (ws: string, id: number, cid: number, reason: string) =>
    fetchJSON<{ comment: Comment }>(`/api/v1/w/${ws}/issues/${id}/comments/${cid}/hide`, {
      method: 'POST',
      ...json({ reason }),
    }),
  unhideComment: (ws: string, id: number, cid: number) =>
    fetchJSON<{ comment: Comment }>(`/api/v1/w/${ws}/issues/${id}/comments/${cid}/unhide`, {
      method: 'POST',
      ...json({}),
    }),
  getIssueMarkdown: (ws: string, id: number) => fetchText(`/api/v1/w/${ws}/issues/${id}/markdown`),
  listLabels: (ws: string, opts: { kind?: string; archived?: boolean } = {}) =>
    fetchJSON<{ labels: Label[] }>(
      `/api/v1/w/${ws}/labels${qs({ kind: opts.kind, archived: opts.archived ? 1 : undefined })}`,
    ),
  createLabel: (ws: string, input: LabelInput) =>
    fetchJSON<{ label: Label }>(`/api/v1/w/${ws}/labels`, { method: 'POST', ...json(input) }),
  updateLabel: (ws: string, id: number, input: { name?: string; color?: string }) =>
    fetchJSON<{ label: Label }>(`/api/v1/w/${ws}/labels/${id}`, {
      method: 'PATCH',
      ...json(input),
    }),
  // DELETE archives the label; restoring is a re-POST of the same
  // name/kind/scope (the store revives the archived row, updating color).
  archiveLabel: (ws: string, id: number) =>
    fetchJSON<void>(`/api/v1/w/${ws}/labels/${id}`, { method: 'DELETE' }),
  listDiscussionCategories: () =>
    fetchJSON<{ categories: DiscussionCategory[] }>('/api/v1/discussion-categories'),
  listDiscussions: (ws: string, f: DiscussionListFilter) =>
    fetchJSON<DiscussionList>(
      `/api/v1/w/${ws}/discussions${qs({ category: f.category, answered: f.answered, q: f.q, sort: f.sort, page: f.page })}`,
    ),
  getDiscussion: (ws: string, id: number) =>
    fetchJSON<DiscussionDetail>(`/api/v1/w/${ws}/discussions/${id}`),
  createDiscussion: (ws: string, input: DiscussionInput) =>
    fetchJSON<{ discussion: Discussion }>(`/api/v1/w/${ws}/discussions`, {
      method: 'POST',
      ...json(input),
    }),
  updateDiscussion: (ws: string, id: number, input: Partial<DiscussionInput>) =>
    fetchJSON<{ discussion: Discussion }>(`/api/v1/w/${ws}/discussions/${id}`, {
      method: 'PATCH',
      ...json(input),
    }),
  deleteDiscussion: (ws: string, id: number) =>
    fetchJSON<void>(`/api/v1/w/${ws}/discussions/${id}`, { method: 'DELETE' }),
  getDiscussionMarkdown: (ws: string, id: number) =>
    fetchText(`/api/v1/w/${ws}/discussions/${id}/markdown`),
  createThread: (ws: string, id: number, body: string) =>
    fetchJSON<{ thread: DiscussionThread }>(`/api/v1/w/${ws}/discussions/${id}/threads`, {
      method: 'POST',
      ...json({ body }),
    }),
  updateThread: (ws: string, id: number, tid: number, body: string) =>
    fetchJSON<{ thread: DiscussionThread }>(`/api/v1/w/${ws}/discussions/${id}/threads/${tid}`, {
      method: 'PATCH',
      ...json({ body }),
    }),
  deleteThread: (ws: string, id: number, tid: number) =>
    fetchJSON<void>(`/api/v1/w/${ws}/discussions/${id}/threads/${tid}`, { method: 'DELETE' }),
  setThreadAnswer: (ws: string, id: number, tid: number, answer: boolean) =>
    fetchJSON<{ thread: DiscussionThread }>(
      `/api/v1/w/${ws}/discussions/${id}/threads/${tid}/answer`,
      { method: 'POST', ...json({ answer }) },
    ),
  hideThread: (ws: string, id: number, tid: number, reason: string) =>
    fetchJSON<{ thread: DiscussionThread }>(
      `/api/v1/w/${ws}/discussions/${id}/threads/${tid}/hide`,
      { method: 'POST', ...json({ reason }) },
    ),
  unhideThread: (ws: string, id: number, tid: number) =>
    fetchJSON<{ thread: DiscussionThread }>(
      `/api/v1/w/${ws}/discussions/${id}/threads/${tid}/unhide`,
      { method: 'POST', ...json({}) },
    ),
  createReply: (ws: string, id: number, tid: number, body: string) =>
    fetchJSON<{ reply: DiscussionReply }>(
      `/api/v1/w/${ws}/discussions/${id}/threads/${tid}/comments`,
      { method: 'POST', ...json({ body }) },
    ),
  updateReply: (ws: string, id: number, tid: number, cid: number, body: string) =>
    fetchJSON<{ reply: DiscussionReply }>(
      `/api/v1/w/${ws}/discussions/${id}/threads/${tid}/comments/${cid}`,
      { method: 'PATCH', ...json({ body }) },
    ),
  deleteReply: (ws: string, id: number, tid: number, cid: number) =>
    fetchJSON<void>(`/api/v1/w/${ws}/discussions/${id}/threads/${tid}/comments/${cid}`, {
      method: 'DELETE',
    }),
  hideReply: (ws: string, id: number, tid: number, cid: number, reason: string) =>
    fetchJSON<{ reply: DiscussionReply }>(
      `/api/v1/w/${ws}/discussions/${id}/threads/${tid}/comments/${cid}/hide`,
      { method: 'POST', ...json({ reason }) },
    ),
  unhideReply: (ws: string, id: number, tid: number, cid: number) =>
    fetchJSON<{ reply: DiscussionReply }>(
      `/api/v1/w/${ws}/discussions/${id}/threads/${tid}/comments/${cid}/unhide`,
      { method: 'POST', ...json({}) },
    ),
  // Pull requests are workspace-independent and mount only when the
  // server resolved a git repo (internal/api/pulls.go); on a repo-less
  // deployment every call below 404s.
  pullsAvailable: async () => {
    try {
      await fetchJSON<PullList>('/api/v1/pulls?per_page=1')
      return true
    } catch {
      return false
    }
  },
  listPullRefs: () => fetchJSON<{ refs: PullRef[] }>('/api/v1/pulls/refs'),
  listPulls: (f: PullListFilter = {}) =>
    fetchJSON<PullList>(
      `/api/v1/pulls${qs({ status: f.status, q: f.q, sort: f.sort, page: f.page })}`,
    ),
  getPull: (id: number) => fetchJSON<PullDetail>(`/api/v1/pulls/${id}`),
  createPull: (input: PullInput) =>
    fetchJSON<{ pull_request: PullRequest }>('/api/v1/pulls', {
      method: 'POST',
      ...json(input),
    }),
  updatePull: (id: number, input: { title: string; body: string }) =>
    fetchJSON<{ pull_request: PullRequest }>(`/api/v1/pulls/${id}`, {
      method: 'PATCH',
      ...json(input),
    }),
  deletePull: (id: number) => fetchJSON<void>(`/api/v1/pulls/${id}`, { method: 'DELETE' }),
  setPullStatus: (id: number, status: 'open' | 'closed') =>
    fetchJSON<{ pull_request: PullRequest }>(`/api/v1/pulls/${id}/status`, {
      method: 'POST',
      ...json({ status }),
    }),
  mergePull: (id: number, method: string, message?: string) =>
    fetchJSON<{ pull_request: PullRequest }>(`/api/v1/pulls/${id}/merge`, {
      method: 'POST',
      ...json(message ? { method, message } : { method }),
    }),
  getPullDiff: async (id: number): Promise<PullDiff> => {
    const res = await fetch(`/api/v1/pulls/${id}/diff`)
    if (!res.ok) {
      let apiError: ApiError['error'] | null = null
      try {
        apiError = (await res.json()).error ?? null
      } catch {
        // non-JSON error body
      }
      throw new RequestError(res.status, apiError)
    }
    return {
      text: await res.text(),
      head_sha: res.headers.get('x-anchor-sha') ?? undefined,
      truncated: res.headers.get('x-diff-truncated') === 'true',
    }
  },
  getPullMarkdown: (id: number) => fetchText(`/api/v1/pulls/${id}/markdown`),
  createPRComment: (id: number, input: PRCommentInput) =>
    fetchJSON<{ comment: PRComment }>(`/api/v1/pulls/${id}/comments`, {
      method: 'POST',
      ...json(input),
    }),
  updatePRComment: (id: number, cid: number, body: string) =>
    fetchJSON<{ comment: PRComment }>(`/api/v1/pulls/${id}/comments/${cid}`, {
      method: 'PATCH',
      ...json({ body }),
    }),
  deletePRComment: (id: number, cid: number) =>
    fetchJSON<void>(`/api/v1/pulls/${id}/comments/${cid}`, { method: 'DELETE' }),
  hidePRComment: (id: number, cid: number, reason: string) =>
    fetchJSON<{ comment: PRComment }>(`/api/v1/pulls/${id}/comments/${cid}/hide`, {
      method: 'POST',
      ...json({ reason }),
    }),
  unhidePRComment: (id: number, cid: number) =>
    fetchJSON<{ comment: PRComment }>(`/api/v1/pulls/${id}/comments/${cid}/unhide`, {
      method: 'POST',
      ...json({}),
    }),
  listWorkspaces: () => fetchJSON<{ workspaces: Workspace[] }>('/api/v1/workspaces'),
  createWorkspace: (input: WorkspaceInput) =>
    fetchJSON<{ workspace: Workspace }>('/api/v1/workspaces', {
      method: 'POST',
      ...json(input),
    }),
  deleteWorkspace: (slug: string) =>
    fetchJSON<void>(`/api/v1/workspaces/${slug}`, { method: 'DELETE' }),
}

// The fixed set of valid hide reasons (internal/store/moderation.go).
export const HIDE_REASONS = [
  'spam',
  'abuse',
  'off-topic',
  'outdated',
  'duplicate',
  'resolved',
  'low-quality',
] as const
