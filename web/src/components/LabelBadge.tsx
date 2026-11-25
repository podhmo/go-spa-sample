import type { Label } from '../api'
import { labelStyle } from '../format'

export function LabelBadge({ label }: { label: Label }) {
  return (
    <span className="label-badge" style={labelStyle(label)}>
      {label.name}
    </span>
  )
}
