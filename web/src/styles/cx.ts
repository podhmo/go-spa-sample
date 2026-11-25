import primitiveStyles from './primitives.module.css'
import contentStyles from './content.module.css'

const styles = { ...primitiveStyles, ...contentStyles }

// Shared classes are intentionally keyed by their original names so call sites
// stay readable while the actual class names remain CSS-module scoped.
export function cx(names: string) {
  return names
    .split(/\s+/)
    .filter(Boolean)
    .map((name) => {
      const scoped = (styles as Record<string, string | undefined>)[name]
      if (scoped === undefined) throw new Error(`unknown shared CSS class: ${name}`)
      return scoped
    })
    .join(' ')
}
