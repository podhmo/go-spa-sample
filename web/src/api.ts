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

export type ApiError = { error: { message: string; fields?: Record<string, string> } }

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

export type IssueListFilter = {
  status?: string
  q?: string
  label?: string
  sort?: string
  page?: number
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

// Single-workspace shortcut for now — the seeded "default" workspace.
// Multi-workspace routing (/w/$ws/...) is a later decision.
export const WS = 'default'
