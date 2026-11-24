import { useSyncExternalStore } from 'react'
import { en, type Messages } from './i18n/en'
export type { Messages }
import { ja } from './i18n/ja'

export type Locale = 'en' | 'ja'

// The MPA's `lang` cookie, mapped to the SPA: localStorage persists the
// explicit choice, navigator.language is the Accept-Language analog,
// English is the default (docs/sketch/ja/i18n.md).
const STORAGE_KEY = 'lang'

const detect = (): Locale => {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === 'en' || saved === 'ja') return saved
  } catch {
    // storage unavailable (private mode, disabled) — fall through
  }
  return navigator.language.toLowerCase().startsWith('ja') ? 'ja' : 'en'
}

let current: Locale = detect()
const listeners = new Set<() => void>()
const subscribe = (onStoreChange: () => void) => {
  listeners.add(onStoreChange)
  return () => {
    listeners.delete(onStoreChange)
  }
}

export const getLocale = (): Locale => current

export const setLocale = (l: Locale) => {
  if (l === current) return
  current = l
  try {
    localStorage.setItem(STORAGE_KEY, l)
  } catch {
    // persistence unavailable — the in-memory switch still works
  }
  document.documentElement.lang = l
  for (const cb of listeners) cb()
}

document.documentElement.lang = current

const useLocale = (): Locale => useSyncExternalStore(subscribe, () => current)

// The active message table. Components read `const m = useMessages()`
// and render `m.section.key` — no context provider needed.
export const useMessages = (): Messages => (useLocale() === 'ja' ? ja : en)

// "{name}" placeholder expansion — the only templating the UI needs.
export const fmt = (template: string, args: Record<string, string | number>): string =>
  template.replace(/\{(\w+)\}/g, (m, key) => (key in args ? String(args[key]) : m))

// fmtTime, locale-aware: en "Jan 2, 2006 15:04", ja "2006年1月2日 15:04"
// (the MPA's fmtTimeFor). Components bind it via `useFmtTime()` so a
// locale switch re-renders timestamps too.
const MONTHS_EN = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
]

export const fmtTimeFor = (locale: Locale, iso: string): string => {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  if (locale === 'ja') {
    return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 ${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  return `${MONTHS_EN[d.getMonth()]} ${d.getDate()}, ${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export const useFmtTime = () => {
  const l = useLocale()
  return (iso: string) => fmtTimeFor(l, iso)
}
