import styles from './primitives.module.css'

// Migration helper: scoped primitive classes keep their original global
// names as compatibility hooks while list/detail selectors are modularized.
// The compat half is removed once every cross-component selector is owned.
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
