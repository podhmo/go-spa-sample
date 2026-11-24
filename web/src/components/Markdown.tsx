import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { remarkAlert } from 'remark-github-blockquote-alert'

// GFM (tables, task lists, strikethrough, autolinks) + GitHub-style
// [!NOTE] alerts, matching the MPA's goldmark setup. Rendering stays in
// the vdom: the markdown is compiled to hast and rendered as React
// elements, so no dangerouslySetInnerHTML is involved. Raw HTML in the
// source is rendered as escaped text (react-markdown's default —
// remark-rehype's allowDangerousHtml produces `raw` hast nodes that
// react-markdown flattens to text). Unsafe link schemes such as
// javascript: are neutralized structurally by the default urlTransform,
// which drops the href of any non-safe protocol.
const remarkPlugins = [remarkGfm, remarkAlert]

export function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown-body">
      <ReactMarkdown remarkPlugins={remarkPlugins}>{text}</ReactMarkdown>
    </div>
  )
}
