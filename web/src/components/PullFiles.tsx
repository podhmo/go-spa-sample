import { useReducer } from 'react'
import type { PRCommentInput } from '../api'
import { submitOnCtrlEnter } from '../enter-submit'

// Parsed unified diff, one row per source line.
type DiffLine = {
  kind: 'ctx' | 'add' | 'del' | 'meta'
  text: string
  // Path + line number this row anchors a review comment to (right = new
  // side, left = old side). Undefined on hunk/file headers.
  path?: string
  line?: number
  side?: 'left' | 'right'
}

type DiffFile = { path: string; oldPath?: string; lines: DiffLine[] }

// parseDiff splits `git diff`-style output into per-file hunks and maps
// every context/add/del row to its anchorable (path, line, side). The
// server wants the post-image path and the line number on that side.
// unquoteGitPath decodes git's C-style path quoting ("a/p\303\251")
// back to the real path; unquoted input passes through unchanged.
function unquoteGitPath(p: string): string {
  if (p.length < 2 || !p.startsWith('"')) return p
  const body = p.slice(1, -1)
  const bytes: number[] = []
  let out = ''
  const flush = () => {
    if (bytes.length === 0) return
    try {
      out += decodeURIComponent(bytes.map((b) => '%' + b.toString(16).padStart(2, '0')).join(''))
    } catch {
      // Non-UTF-8 bytes (a latin-1 filename, etc.) can't decode — render
      // the bytes lossily rather than failing the whole Files tab.
      out += String.fromCharCode(...bytes)
    }
    bytes.length = 0
  }
  for (let i = 0; i < body.length; i++) {
    const ch = body[i]!
    if (ch !== '\\') {
      flush()
      out += ch
      continue
    }
    const oct = /^[0-7]{1,3}/.exec(body.slice(i + 1, i + 4))
    if (oct) {
      bytes.push(parseInt(oct[0], 8))
      i += oct[0].length
    } else {
      flush()
      const e = body[++i] ?? ''
      out += e === 'n' ? '\n' : e === 't' ? '\t' : e
    }
  }
  flush()
  return out
}

// diffSide reads a `--- a/x` / `+++ b/x` operand, quoted or not.
// Unquoted operands may end with a separator tab (worktree diffs add a
// timestamp after it) or a marker space for names ending in blanks.
function diffSide(rest: string): string | undefined {
  if (rest === '/dev/null') return undefined
  const q = /^"[ab]\/(?:\\.|[^"])*"$/.exec(rest)
  if (q) return unquoteGitPath(q[0]).slice(2)
  const u = /^[ab]\/(.+?)\s*$/.exec(rest)
  return u?.[1]
}

function parseDiff(text: string): DiffFile[] {
  const files: DiffFile[] = []
  let cur: DiffFile | undefined
  let path = ''
  let oldNo = 0
  let newNo = 0
  let inHunk = false
  // One trailing newline is an artifact of the diff text itself; blank
  // context lines inside a hunk are emitted as bare empty lines and
  // must still consume both counters.
  for (const raw of text.replace(/\n$/, '').split('\n')) {
    if (raw.startsWith('diff --git ')) {
      const m = /^diff --git ("a\/(?:\\.|[^"])*"|a\/.+) ("b\/(?:\\.|[^"])*"|b\/.+)$/.exec(raw)
      const b = m?.[2]
      path = b
        ? b.startsWith('"')
          ? unquoteGitPath(b).slice(2)
          : b.slice(2).replace(/\s+$/, '')
        : ''
      cur = { path, lines: [] }
      files.push(cur)
      inHunk = false
      continue
    }
    if (!cur) continue
    if (raw.startsWith('@@ ')) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(raw)
      if (m) {
        oldNo = Number(m[1])
        newNo = Number(m[2])
      }
      inHunk = true
      cur.lines.push({ kind: 'meta', text: raw })
      continue
    }
    // Between "diff --git" and the first "@@" sit file headers (index,
    // modes, ---/+++ paths) — render them, but they anchor nothing.
    // The ---/+++ paths are authoritative over the diff --git line:
    // quoted names parse cleanly here and renamed files get both sides.
    if (!inHunk) {
      if (raw.startsWith('--- ')) {
        const p = diffSide(raw.slice(4))
        if (p !== undefined) cur.oldPath = p
      }
      if (raw.startsWith('+++ ')) {
        const p = diffSide(raw.slice(4))
        if (p !== undefined) {
          path = p
          cur.path = p
        }
      }
      if (raw !== '') cur.lines.push({ kind: 'meta', text: raw })
      continue
    }
    if (raw.startsWith('+')) {
      cur.lines.push({ kind: 'add', text: raw, path, line: newNo, side: 'right' })
      newNo++
      continue
    }
    if (raw.startsWith('-')) {
      cur.lines.push({
        kind: 'del',
        text: raw,
        path: cur.oldPath ?? path,
        line: oldNo,
        side: 'left',
      })
      oldNo++
      continue
    }
    // "\\ No newline" markers are hunk metadata, not content rows.
    if (raw.startsWith('\\') || (raw !== '' && !raw.startsWith(' '))) {
      cur.lines.push({ kind: 'meta', text: raw })
      continue
    }
    cur.lines.push({ kind: 'ctx', text: raw, path, line: newNo, side: 'right' })
    oldNo++
    newNo++
  }
  return files
}

type FormUI = { openAt: string | null }
type FormAction = { type: 'toggle'; key: string | null }

// The files tab: unified diff with a comment button on every real line,
// expanding an inline form that posts {body, path, line, side}.
export function PullFiles({
  diff,
  pending,
  onComment,
}: {
  diff: string
  pending?: boolean | undefined
  // Rejected posts propagate: DiffRow keeps the form open with the
  // draft and shows the API's error (e.g. a stale anchor after a
  // force-push).
  onComment?: (input: PRCommentInput) => Promise<void> | void
}) {
  const [ui, dispatch] = useReducer(
    (s: FormUI, a: FormAction): FormUI => ({ openAt: s.openAt === a.key ? null : a.key }),
    { openAt: null },
  )
  const files = parseDiff(diff)
  if (files.length === 0) {
    return <p className="muted">No changes.</p>
  }
  return (
    <div className="diff-files">
      {files.map((f) => (
        <section className="diff-file" key={f.path}>
          <h2 className="diff-file-name">
            <code>{f.path}</code>
          </h2>
          <table className="diff-table">
            <tbody>
              {f.lines.map((l, i) => {
                const key = `${f.path}:${l.side}:${l.line ?? i}`
                const formOpen = ui.openAt === key
                return (
                  <DiffRow
                    key={key}
                    line={l}
                    formOpen={formOpen}
                    pending={pending}
                    canComment={onComment !== undefined && l.path !== undefined}
                    onToggle={() => dispatch({ type: 'toggle', key })}
                    onSubmit={async (body) => {
                      if (l.path && l.line && l.side) {
                        await onComment?.({ body, path: l.path, line: l.line, side: l.side })
                      }
                    }}
                  />
                )
              })}
            </tbody>
          </table>
        </section>
      ))}
    </div>
  )
}

function DiffRow({
  line,
  formOpen,
  pending,
  canComment,
  onToggle,
  onSubmit,
}: {
  line: DiffLine
  formOpen: boolean
  pending?: boolean | undefined
  canComment: boolean
  onToggle: () => void
  // Resolves when the comment posted; rejection keeps the draft and
  // shows the error.
  onSubmit: (body: string) => Promise<void> | void
}) {
  const [postError, setPostError] = useReducer((_: string | null, v: string | null) => v, null)
  if (line.kind === 'meta') {
    return (
      <tr className="diff-line diff-meta">
        <td className="diff-comment-cell" />
        <td colSpan={2}>
          <code>{line.text}</code>
        </td>
      </tr>
    )
  }
  return (
    <>
      <tr className={`diff-line diff-${line.kind}`}>
        <td className="diff-comment-cell">
          {canComment && (
            <button
              type="button"
              className="diff-add-comment"
              onClick={onToggle}
              aria-expanded={formOpen}
              aria-label={`Comment on ${line.path} line ${line.line} (${line.side} side)`}
            >
              +
            </button>
          )}
        </td>
        <td className="diff-lineno">
          <code>{line.line}</code>
        </td>
        <td className="diff-text">
          <code>{line.text}</code>
        </td>
      </tr>
      {formOpen && (
        <tr className="diff-line diff-comment-form">
          <td />
          <td colSpan={2}>
            <form
              aria-label={`comment on ${line.path} line ${line.line}`}
              onSubmit={(e) => {
                e.preventDefault()
                const body = String(new FormData(e.currentTarget).get('body') ?? '')
                setPostError(null)
                void (async () => {
                  try {
                    await onSubmit(body)
                    onToggle()
                  } catch (err) {
                    setPostError(err instanceof Error ? err.message : 'Comment failed.')
                  }
                })()
              }}
            >
              <div className="field">
                <label
                  htmlFor={`dc-${line.path}-${line.side}-${line.line}`}
                  className="visually-hidden"
                >
                  comment
                </label>
                <textarea
                  id={`dc-${line.path}-${line.side}-${line.line}`}
                  name="body"
                  rows={3}
                  required
                  onKeyDown={submitOnCtrlEnter}
                  data-error={`dc-${line.path}-${line.line}-error`}
                  placeholder="Leave a review comment…"
                />
                <span id={`dc-${line.path}-${line.line}-error`} className="error-msg">
                  Body is required.
                </span>
                {postError && <em role="alert">{postError}</em>}
              </div>
              <div className="form-actions">
                <button type="button" className="btn btn-quiet" onClick={onToggle}>
                  Cancel
                </button>
                <button type="submit" className="btn btn-primary" disabled={pending}>
                  Comment
                </button>
              </div>
            </form>
          </td>
        </tr>
      )}
    </>
  )
}
