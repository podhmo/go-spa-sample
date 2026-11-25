// Parsed unified diff, one row per source line.
export type DiffLine = {
  kind: 'ctx' | 'add' | 'del' | 'meta'
  text: string
  // Path + line number this row anchors a review comment to (right = new
  // side, left = old side). Undefined on hunk/file headers.
  path?: string
  line?: number
  side?: 'left' | 'right'
}

export type DiffFile = { path: string; oldPath?: string; lines: DiffLine[] }

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

export function parseDiff(text: string): DiffFile[] {
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
