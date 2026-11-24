// Live updates: subscribe to the server's SSE hint stream and invalidate
// the router on every hint. Events carry no content ({seq, kind, target,
// parent}) — they only say "something changed", and invalidation
// re-fetches the active loaders, so additions, edits, hides and deletes
// from any writer (including the local CLI writing the database in its
// own process) show up on open issue/discussion screens.
//
// Two constraints shape the wiring:
//
// - HTTP/1.1 browsers cap simultaneous connections per origin (6 in
//   Chrome). Each open tab holding an SSE stream spends one, so a
//   handful of background tabs could starve actual API requests. Only
//   the visible tab holds a stream; a tab that becomes visible catches
//   up with one invalidate.
// - EventSource does not retry fatal openings (a 401 before login
//   closes it for good), so reconnects are driven manually: onerror
//   with readyState CLOSED re-creates the source after a delay, which
//   is also what picks the stream up after a server restart or a sign-in
//   that needed no page reload.

import type { AnyRouter } from '@tanstack/react-router'

const reconnectMs = 3000
const debounceMs = 150

export function watchLiveUpdates(router: AnyRouter, url = '/api/v1/events'): () => void {
  let es: EventSource | undefined
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let debounceTimer: ReturnType<typeof setTimeout> | undefined
  let stopped = false

  const invalidate = () => {
    clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => {
      void router.invalidate()
    }, debounceMs)
  }

  const close = () => {
    es?.close()
    es = undefined
  }

  const connect = () => {
    // A retry timer can outlive the tab going hidden; opening in the
    // background would spend a per-origin connection nobody watches.
    if (stopped || es || document.visibilityState !== 'visible') return
    es = new EventSource(url)
    es.onmessage = invalidate
    es.onerror = () => {
      if (es?.readyState === EventSource.CLOSED) {
        close()
        clearTimeout(retryTimer)
        retryTimer = setTimeout(connect, reconnectMs)
      }
      // readyState CONNECTING means the browser is already retrying on
      // its own — leave it alone.
    }
  }

  const onVisibility = () => {
    if (document.visibilityState === 'visible') {
      connect()
      // Hints that fired while the tab was hidden are gone; one
      // invalidation resyncs whatever the loaders hold.
      invalidate()
    } else {
      close()
    }
  }

  document.addEventListener('visibilitychange', onVisibility)
  onVisibility()

  return () => {
    stopped = true
    document.removeEventListener('visibilitychange', onVisibility)
    clearTimeout(retryTimer)
    clearTimeout(debounceTimer)
    close()
  }
}
