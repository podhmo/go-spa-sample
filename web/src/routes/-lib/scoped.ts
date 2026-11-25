import { notFound } from '@tanstack/react-router'
import { RequestError, api } from '../../api'

// A scoped API call: a plain 404 becomes the router's notFound, while a
// cross-scope 404 names the owning workspace in error.workspace — the
// API's in-band counterpart of the MPA's 302 — and rebase() throws a
// redirect to the same page under that slug.
export const scoped = async <T>(p: Promise<T>, rebase: (real: string) => never): Promise<T> => {
  try {
    return await p
  } catch (err) {
    if (err instanceof RequestError && err.status === 404) {
      const real = err.apiError?.workspace
      if (typeof real === 'string') rebase(real)
      throw notFound()
    }
    throw err
  }
}

// Where a workspace-less URL lands: 'default' when it exists, else the
// first workspace (the MPA's rule); null when none exist at all.
export const homeSlug = async (): Promise<string | null> => {
  const { workspaces } = await api.listWorkspaces()
  return (workspaces.find((w) => w.slug === 'default') ?? workspaces[0])?.slug ?? null
}
