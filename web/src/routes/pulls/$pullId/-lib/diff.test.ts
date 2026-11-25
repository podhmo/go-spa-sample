import { describe, expect, it } from 'vitest'
import { parseDiff } from './diff'

describe('parseDiff', () => {
  it('anchors additions, deletions, and context to their line numbers', () => {
    const [file] = parseDiff(
      [
        'diff --git a/a.txt b/a.txt',
        '--- a/a.txt',
        '+++ b/a.txt',
        '@@ -2,2 +2,3 @@',
        ' ctx',
        '-old',
        '+new',
        '+next',
      ].join('\n'),
    )
    expect(file?.lines.filter((l) => l.kind !== 'meta')).toEqual([
      { kind: 'ctx', text: ' ctx', path: 'a.txt', line: 2, side: 'right' },
      { kind: 'del', text: '-old', path: 'a.txt', line: 3, side: 'left' },
      { kind: 'add', text: '+new', path: 'a.txt', line: 3, side: 'right' },
      { kind: 'add', text: '+next', path: 'a.txt', line: 4, side: 'right' },
    ])
  })

  it('decodes quoted paths and keeps deletion anchors on the old rename path', () => {
    const [file] = parseDiff(
      [
        'diff --git "a/caf\\303\\251.txt" "b/caf\\303\\251-renamed.txt"',
        '--- "a/caf\\303\\251.txt"',
        '+++ "b/caf\\303\\251-renamed.txt"',
        '@@ -1 +1 @@',
        '-old',
        '+new',
      ].join('\n'),
    )
    expect(file?.path).toBe('café-renamed.txt')
    expect(file?.oldPath).toBe('café.txt')
    expect(file?.lines.find((l) => l.kind === 'del')?.path).toBe('café.txt')
    expect(file?.lines.find((l) => l.kind === 'add')?.path).toBe('café-renamed.txt')
  })
})
