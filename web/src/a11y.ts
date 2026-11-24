// Keeps aria-invalid / aria-describedby in sync with the visual :user-invalid
// state, and announces field errors via one polite live region.
// Adapted from modern-web-guidance "accessible-error-announcement".

const linkError = (el: HTMLElement, shown: boolean) => {
  const errorId = el.dataset.error
  if (!errorId) return
  const ids = (el.getAttribute('aria-describedby') || '')
    .split(/\s+/)
    .filter((id) => id && id !== errorId)
  if (shown) ids.push(errorId)
  if (ids.length) el.setAttribute('aria-describedby', ids.join(' '))
  else el.removeAttribute('aria-describedby')
}

let announcer: HTMLDivElement | undefined
let announceTimer: ReturnType<typeof setTimeout> | undefined
let announcedField: HTMLElement | undefined
const announced = new WeakSet<Element>()
const flagged = new WeakSet<Element>()

const spokenText = (node: Element): string => {
  const copy = node.cloneNode(true) as Element
  copy.querySelectorAll('[aria-hidden="true"]').forEach((n) => n.remove())
  return (copy.textContent ?? '').replace(/\s+/g, ' ').trim()
}

const announceError = (el: HTMLElement) => {
  const error = document.getElementById(el.dataset.error ?? '')
  if (!error || !announcer) return
  const label = (el as HTMLInputElement).labels?.[0]
  const prefix = label ? `${spokenText(label)}: ` : ''
  clearTimeout(announceTimer)
  announcer.textContent = ''
  announcedField = el
  announceTimer = setTimeout(() => {
    if (announcer) announcer.textContent = prefix + spokenText(error)
  }, 500)
}

const syncAria = (target: EventTarget | null, announce = false) => {
  const el = target as HTMLElement | null
  if (!el?.matches?.('input, select, textarea')) return
  if (el.matches(':user-invalid')) {
    if (announce && !announced.has(el)) {
      announceError(el)
      announced.add(el)
    }
    el.setAttribute('aria-invalid', 'true')
    linkError(el, true)
    flagged.add(el)
  } else if (flagged.has(el) || el.getAttribute('aria-invalid') !== 'true') {
    // An aria-invalid="true" we didn't set (e.g. a server error) is left alone.
    el.setAttribute('aria-invalid', 'false')
    linkError(el, false)
    flagged.delete(el)
    announced.delete(el)
    if (announcedField === el && announcer) {
      clearTimeout(announceTimer)
      announcer.textContent = ''
    }
  }
}

let initialized = false
export function setupFieldErrorSync() {
  if (initialized) return
  initialized = true

  announcer = document.createElement('div')
  announcer.setAttribute('aria-live', 'polite')
  announcer.classList.add('visually-hidden')
  document.body.append(announcer)

  document.addEventListener('blur', (e) => syncAria(e.target, true), true)
  document.addEventListener('input', (e) => {
    const t = e.target as HTMLElement
    if (t?.hasAttribute?.('aria-invalid')) syncAria(t)
  })
  document.addEventListener('change', (e) => syncAria(e.target, true), true)
  document.addEventListener(
    'invalid',
    (e) => {
      if (e.target) announced.add(e.target as Element)
      syncAria(e.target)
    },
    true,
  )
  document.addEventListener(
    'reset',
    (e) => {
      const form = e.target as HTMLFormElement
      setTimeout(() => [...form.elements].forEach((el) => syncAria(el)))
    },
    true,
  )
}
