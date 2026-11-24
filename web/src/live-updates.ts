// Live updates: subscribe to the server's SSE hint stream and invalidate
// the router on every hint. Events carry no content ({seq, kind, target,
// parent}) — they only say "something changed", and invalidation
// re-fetches the active loaders, so additions, edits, hides and deletes
// from any writer (including the local CLI writing the database in its
// own process) show up on open issue/discussion screens.
//
// Reconnects are EventSource's job (the server sends retry: 3000); a
// burst of hints collapses into one invalidation per quiet window
// because loaders dedupe concurrent fetches anyway.

import type { AnyRouter } from '@tanstack/react-router'

export function watchLiveUpdates(router: AnyRouter, url = '/api/v1/events'): () => void {
  const es = new EventSource(url)
  let timer: ReturnType<typeof setTimeout> | undefined
  es.onmessage = () => {
    // Trailing-edge debounce: a CLI writing several rows in one go
    // (thread + replies) invalidates once, not per row.
    clearTimeout(timer)
    timer = setTimeout(() => {
      void router.invalidate()
    }, 150)
  }
  return () => {
    clearTimeout(timer)
    es.close()
  }
}
