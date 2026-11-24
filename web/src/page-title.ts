import { useEffect } from 'react'
import { fmt, type Messages } from './i18n'

// document.title is something full-page loads gave the MPA for free;
// the SPA derives it from the matched path. Detail pages only carry
// ids (re-fetching the entity title would duplicate the loader).
export function pageTitle(pathname: string, m: Messages): string {
  const rest = pathname.replace(/^\/w\/[^/]+/, '') || '/'
  const [section, id, sub] = rest.split('/').filter(Boolean)
  let t: string
  switch (section) {
    case 'issues':
      t = !id
        ? m.issues.title
        : id === 'new'
          ? m.issues.new
          : sub === 'edit'
            ? fmt(m.issues.editTitle, { id })
            : `${m.issues.title} #${id}`
      break
    case 'discussions':
      t = !id
        ? m.discussions.title
        : id === 'new'
          ? m.discussions.new
          : sub === 'edit'
            ? fmt(m.discussions.editTitle, { id })
            : `${m.discussions.title} #${id}`
      break
    case 'pulls':
      t = !id
        ? m.pulls.title
        : id === 'new'
          ? m.pulls.new
          : sub === 'edit'
            ? fmt(m.pulls.editTitle, { id })
            : sub === 'files'
              ? `${m.pulls.title} #${id} — ${m.pulls.files}`
              : `${m.pulls.title} #${id}`
      break
    case 'workspaces':
      t = id === 'new' ? m.workspaces.new : m.workspaces.title
      break
    case 'labels':
      t = m.labels.title
      break
    case 'login':
      t = m.nav.logIn
      break
    case 'register':
      t = m.nav.register
      break
    default:
      return 'zatudan'
  }
  return `${t} — zatudan`
}

export function usePageTitle(pathname: string, m: Messages) {
  useEffect(() => {
    document.title = pageTitle(pathname, m)
  }, [pathname, m])
}
