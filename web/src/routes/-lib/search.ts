// Keys are always present in the validated/search-prop objects but may
// hold undefined (exactOptionalPropertyTypes), so each is `T | undefined`.
export type IssueSearch = {
  status?: string | undefined
  q?: string | undefined
  label?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

export type DiscussionSearch = {
  category?: string | undefined
  answered?: string | undefined
  q?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

export type PullSearch = {
  status?: string | undefined
  q?: string | undefined
  sort?: string | undefined
  page?: number | undefined
}

// TanStack Router decodes `?q=404`-style values to numbers and
// `?q=true` to booleans — coerce those back so numeric keywords survive.
export const str = (v: unknown) =>
  typeof v === 'string' && v !== ''
    ? v
    : typeof v === 'number' && Number.isFinite(v)
      ? String(v)
      : typeof v === 'boolean'
        ? String(v)
        : undefined

export const num = (v: unknown) => {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number(v) : NaN
  return Number.isInteger(n) && n >= 1 ? n : undefined
}

export const issueSearch = (search: Record<string, unknown>): IssueSearch => ({
  status: str(search.status),
  q: str(search.q),
  label: str(search.label),
  sort: str(search.sort),
  page: num(search.page),
})

export const discussionSearch = (search: Record<string, unknown>): DiscussionSearch => ({
  category: str(search.category),
  answered: str(search.answered),
  q: str(search.q),
  sort: str(search.sort),
  page: num(search.page),
})

export const pullSearch = (search: Record<string, unknown>): PullSearch => ({
  status: str(search.status),
  q: str(search.q),
  sort: str(search.sort),
  page: num(search.page),
})

export const authSearch = (search: Record<string, unknown>) => ({ next: str(search.next) })

// Same-site redirect target after login/registration: absolute paths
// only — '//evil' is a protocol-relative URL and some browsers treat
// '\' as '/', both escapes a naive startsWith('/') check would miss.
export const safeNext = (raw: string | undefined): string =>
  raw !== undefined && raw.startsWith('/') && !raw.startsWith('//') && !raw.includes('\\')
    ? raw
    : '/'
