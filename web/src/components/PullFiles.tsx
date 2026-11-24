import { useEffect, useReducer, useRef } from 'react'
import { useLeaveGuard } from '../leave-guard'
import { RequestError, type PRCommentInput } from '../api'
import { submitOnCtrlEnter } from '../enter-submit'
import { fmt, useMessages } from '../i18n'

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
// Unquoted operands end at the first tab: git appends one after names
// that end in a blank (and GNU diff before a timestamp), while names
// with real tabs are quoted — so spaces are kept, tabs are cut.
function diffSide(rest: string): string | undefined {
  if (rest === '/dev/null') return undefined
  const q = /^"[ab]\/(?:\\.|[^"])*"$/.exec(rest)
  if (q) return unquoteGitPath(q[0]).slice(2)
  const u = /^[ab]\/([^\t]*)/.exec(rest)
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
      const m = /^diff --git ("a\/(?:\\.|[^"])*"|a\/.+) ("b\/(?:\\.|[^"])*"|b\/[^\t]+)$/.exec(raw)
      const b = m?.[2]
      path = b ? (b.startsWith('"') ? unquoteGitPath(b).slice(2) : b.slice(2)) : ''
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

// The open draft anchors to a coordinate AND that row's code when the
// form opened (without the +/-/space diff marker — a rebase can turn an
// added line into context without changing its source text). A
// rewritten diff can put different code at the same path:side:line, and
// a retry must not attach the draft to it.
type FormUI = { openAt: { key: string; text: string } | null }
type FormAction = { type: 'toggle'; open: { key: string; text: string } | null }

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
    (s: FormUI, a: FormAction): FormUI => ({
      openAt: s.openAt?.key === a.open?.key ? null : a.open,
    }),
    { openAt: null },
  )
  const files = parseDiff(diff)
  // A different diff (force-push, new commits) can shift every row. An
  // open draft whose anchor row disappeared — or whose row now holds
  // different code — no longer points at the code it was written for:
  // close it rather than let a retry attach it to replacement code.
  useEffect(() => {
    if (ui.openAt === null) return
    const row = files
      .flatMap((f) => f.lines.map((l, i) => ({ key: `${f.path}:${l.side}:${l.line ?? i}`, l })))
      .find((r) => r.key === ui.openAt?.key)
    if (!row || row.l.text.slice(1) !== ui.openAt.text) {
      dispatch({ type: 'toggle', open: null })
    }
    // `files` reparses every render; diff + openAt are the triggers.
    // oxlint-disable-next-line react/exhaustive-deps
  }, [diff, ui.openAt])
  const m = useMessages()
  if (files.length === 0) {
    return <p className="muted">{m.pulls.noChanges}</p>
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
                const formOpen = ui.openAt?.key === key
                return (
                  <DiffRow
                    key={key}
                    line={l}
                    formOpen={formOpen}
                    pending={pending}
                    canComment={onComment !== undefined && l.path !== undefined}
                    onToggle={() =>
                      dispatch({ type: 'toggle', open: { key, text: l.text.slice(1) } })
                    }
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
  const [dirty, setDirty] = useReducer((_: boolean, v: boolean) => v, false)
  const [blank, setBlank] = useReducer((_: boolean, v: boolean) => v, true)
  const m = useMessages()
  useLeaveGuard(formOpen && dirty, m.form.unsavedChanges)
  // The disabled button only blocks clicks: Ctrl+Enter (requestSubmit)
  // still fires, so the send itself must be re-entrant-safe.
  const sending = useRef(false)
  // The textarea is uncontrolled, so closing the form discards the DOM
  // draft: every toggle must reset the flags or a reopen shows an empty
  // box with Comment enabled / a phantom dirty guard.
  const toggle = () => {
    setDirty(false)
    setBlank(true)
    setPostError(null)
    onToggle()
  }
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
              onClick={toggle}
              aria-expanded={formOpen}
              aria-label={fmt(m.pulls.commentOnLine, {
                path: line.path ?? '',
                line: line.line ?? 0,
                side: line.side ?? '',
              })}
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
              aria-label={fmt(m.pulls.commentOnLineAria, {
                path: line.path ?? '',
                line: line.line ?? 0,
              })}
              onSubmit={(e) => {
                e.preventDefault()
                if (sending.current) return
                sending.current = true
                const body = String(new FormData(e.currentTarget).get('body') ?? '')
                setPostError(null)
                // Clear the guard before awaiting: the route navigates
                // to the new comment inside onSubmit, and a still-dirty
                // flag would intercept that redirect.
                setDirty(false)
                void (async () => {
                  try {
                    await onSubmit(body)
                    toggle()
                  } catch (err) {
                    // The draft is still in the textarea — re-arm.
                    setDirty(true)
                    // Field errors name the actual rejection reason
                    // (e.g. a stale anchor); a bare message just says
                    // "Validation failed.".
                    const fields = err instanceof RequestError ? err.apiError?.fields : undefined
                    setPostError(
                      fields
                        ? Object.values(fields).join(' ')
                        : err instanceof Error
                          ? err.message
                          : m.pulls.commentFailed,
                    )
                  } finally {
                    sending.current = false
                  }
                })()
              }}
            >
              <div className="field">
                <label
                  htmlFor={`dc-${line.path}-${line.side}-${line.line}`}
                  className="visually-hidden"
                >
                  {m.pulls.comment}
                </label>
                <textarea
                  id={`dc-${line.path}-${line.side}-${line.line}`}
                  name="body"
                  rows={3}
                  required
                  onChange={(e) => {
                    setDirty(e.target.value.trim() !== '')
                    setBlank(e.target.value.trim() === '')
                  }}
                  onKeyDown={submitOnCtrlEnter}
                  data-error={`dc-${line.path}-${line.line}-error`}
                  placeholder={m.pulls.reviewCommentPh}
                />
                <span id={`dc-${line.path}-${line.line}-error`} className="error-msg">
                  {m.form.bodyRequired}
                </span>
                {postError && <em role="alert">{postError}</em>}
              </div>
              <div className="form-actions">
                <button type="button" className="btn btn-quiet" onClick={toggle}>
                  {m.copy.cancel}
                </button>
                <button type="submit" className="btn btn-primary" disabled={pending || blank}>
                  {m.pulls.commentSubmit}
                </button>
              </div>
            </form>
          </td>
        </tr>
      )}
    </>
  )
}
