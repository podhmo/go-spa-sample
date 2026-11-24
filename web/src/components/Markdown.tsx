import DOMPurify, { type Config } from 'dompurify'
import { marked } from 'marked'
import markedAlert from 'marked-alert'
import { useMemo } from 'react'

const escapeHtml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

// marked GFM + GitHub-style [!NOTE] alerts (markedAlert), matching the
// MPA's goldmark setup. Raw HTML in the source is escaped to text —
// the MPA emits no raw HTML either — and DOMPurify is the second layer
// that also neutralizes unsafe hrefs such as javascript: links.
marked.use(markedAlert(), {
  renderer: {
    html: ({ text }) => escapeHtml(text),
  },
})

// Whitelist of tags markdown can generate (incl. task-list checkboxes,
// GFM tables and marked-alert's alert/icon markup); attributes are
// limited to what those tags legitimately need.
const PURIFY: Config = {
  ALLOWED_TAGS: [
    'a',
    'blockquote',
    'br',
    'code',
    'del',
    'div',
    'em',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'hr',
    'img',
    'input',
    'li',
    'ol',
    'p',
    'path',
    'pre',
    'span',
    'strong',
    'svg',
    'table',
    'tbody',
    'td',
    'th',
    'thead',
    'tr',
    'ul',
  ],
  ALLOWED_ATTR: [
    'alt',
    'aria-hidden',
    'checked',
    'class',
    'colspan',
    'disabled',
    'height',
    'href',
    'src',
    'title',
    'type',
    'viewBox',
    'width',
  ],
}

export function Markdown({ text }: { text: string }) {
  const html = useMemo(
    () => DOMPurify.sanitize(marked.parse(text, { async: false }) as string, PURIFY),
    [text],
  )
  // dangerouslySetInnerHtml is safe here: the markup is produced by
  // marked with raw HTML escaped, then passed through DOMPurify.
  return <div className="markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
}
