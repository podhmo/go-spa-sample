import primitiveStyles from './primitives.module.css'
import contentStyles from './content.module.css'

const styles = { ...primitiveStyles, ...contentStyles }

// Migration helper: scoped shared classes keep their original global
// names as compatibility hooks while cross-component selectors are
// modularized. The compat half is removed once those selectors are owned.
export function cx(names: string) {
  return names
    .split(/\s+/)
    .filter(Boolean)
    .map((name) => {
      const scoped = (styles as Record<string, string | undefined>)[name]
      return scoped === undefined || scoped === name ? name : `${scoped} ${name}`
    })
    .join(' ')
}
