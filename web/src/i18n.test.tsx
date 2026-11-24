import { page } from 'vitest/browser'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { setLocale } from './i18n'
import { renderAt, stubAPI } from './test-utils'

// setLocale pins the store regardless of navigator.language; localStorage
// alone wouldn't — i18n.tsx caches the detected locale at module load.
afterEach(() => {
  setLocale('en')
  localStorage.removeItem('lang')
  vi.unstubAllGlobals()
})

describe('locale switching', () => {
  it('switches nav text to Japanese via the lang switch', async () => {
    setLocale('en')
    stubAPI((url) => {
      if (url.includes('/labels')) return { body: { labels: [] } }
      if (url.includes('/pulls')) return { status: 404, body: {} } // availability probe
      return { body: { issues: [], total: 0, page: 1, per_page: 20 } }
    })
    renderAt('/w/default/issues')
    await expect.element(page.getByText('New issue')).toBeInTheDocument()
    await page.getByLabelText('Language').selectOptions('ja')
    await expect.element(page.getByText('新規 issue')).toBeInTheDocument()
    expect(localStorage.getItem('lang')).toBe('ja')
    expect(document.documentElement.lang).toBe('ja')
  })
})
