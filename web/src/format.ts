import type { CSSProperties } from 'react'
import type { Label } from './api'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

// Mirrors the MPA's fmtTime (English locale): "Jan 2, 2006 15:04" in
// local time — e.g. "Nov 23, 2026 01:43".
export const fmtTime = (iso: string) => {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${MONTHS[d.getMonth()]} ${d.getDate()}, ${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// --label-color is the DB-provided badge background; the stylesheet
// picks black or white text for contrast (guide contrast-color).
export const labelStyle = (label: Label) => ({ '--label-color': label.color }) as CSSProperties
