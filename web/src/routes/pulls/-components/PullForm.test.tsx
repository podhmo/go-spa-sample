import { page } from 'vitest/browser'
import { describe, expect, it, vi } from 'vitest'
import { renderWithRouter } from '../../../test-utils'
import { PullForm } from './PullForm'

const refs = {
  refs: [
    { name: 'refs/heads/main', short: 'main' },
    { name: 'refs/heads/feature-x', short: 'feature-x' },
  ],
}

describe('component: PullForm', () => {
  it('requires branches and a title, then submits refs', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<PullForm onSubmit={onSubmit} refs={refs.refs} />)
    await page.getByLabelText(/Head/).selectOptions('refs/heads/feature-x')
    await page.getByLabelText(/Base/).selectOptions('refs/heads/main')
    await page.getByLabelText(/Title/).fill('my pr')
    await page.getByRole('button', { name: 'Create pull request' }).click()
    expect(onSubmit).toHaveBeenCalledWith({
      title: 'my pr',
      body: '',
      base: 'refs/heads/main',
      head: 'refs/heads/feature-x',
    })
  })

  it('edits title and body only', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<PullForm onSubmit={onSubmit} initial={{ title: 'old', body: 'old body' }} />)
    await page.getByLabelText(/Title/).fill('renamed')
    await page.getByRole('button', { name: 'Save changes' }).click()
    expect(onSubmit).toHaveBeenCalledWith({ title: 'renamed', body: 'old body' })
  })
})
