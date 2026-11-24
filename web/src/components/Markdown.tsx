import { Suspense, lazy } from 'react'

// GFM (tables, task lists, strikethrough, autolinks) + GitHub-style
// [!NOTE] alerts, matching the MPA's goldmark setup. Rendering stays in
// the vdom: the markdown is compiled to hast and rendered as React
// elements, so no dangerouslySetInnerHTML is involved. Raw HTML in the
// source is rendered as escaped text (react-markdown's default —
// remark-rehype's allowDangerousHtml produces `raw` hast nodes that
// react-markdown flattens to text). Unsafe link schemes such as
// javascript: are neutralized structurally by the default urlTransform,
// which drops the href of any non-safe protocol.
//
// The unified/remark tree is the app's heaviest dependency, so it loads
// as a split chunk: the raw source shows until it arrives, then swaps
// to rendered markdown. Plain text as the fallback keeps layout shape
// and stays readable during the one-time chunk fetch.
const LazyMarkdown = lazy(async () => {
  const [{ default: ReactMarkdown }, { default: remarkGfm }, { remarkAlert }] = await Promise.all([
    import('react-markdown'),
    import('remark-gfm'),
    import('remark-github-blockquote-alert'),
  ])
  const remarkPlugins = [remarkGfm, remarkAlert]
  return {
    default: ({ text }: { text: string }) => (
      <ReactMarkdown remarkPlugins={remarkPlugins}>{text}</ReactMarkdown>
    ),
  }
})

export function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown-body">
      <Suspense fallback={text}>
        <LazyMarkdown text={text} />
      </Suspense>
    </div>
  )
}
