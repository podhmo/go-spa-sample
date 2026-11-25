import { page } from 'vitest/browser'
import { describe, expect, it, vi } from 'vitest'
import type { DiscussionCategory } from '../../../../../api'
import { renderWithRouter } from '../../../../../test-utils'
import { DiscussionForm } from './DiscussionForm'

const cat = (over: Partial<DiscussionCategory> = {}): DiscussionCategory => ({
  id: 2,
  name: 'General',
  slug: 'general',
  description: 'Anything',
  format: 'open',
  ...over,
})

const qaCat = cat({ id: 4, name: 'Q&A', slug: 'qa', format: 'qa' })

describe('component: DiscussionForm', () => {
  it('requires a category and submits it', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<DiscussionForm onSubmit={onSubmit} categories={[cat(), qaCat]} />)
    await page.getByLabelText(/Title/).fill('hello discussion')
    await page.getByLabelText(/Category/).selectOptions('4')
    await page.getByRole('button', { name: 'Create discussion' }).click()
    expect(onSubmit).toHaveBeenCalledWith({
      title: 'hello discussion',
      body: '',
      category_id: 4,
      label_ids: [],
    })
  })

  it('rejects submit without a category', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<DiscussionForm onSubmit={onSubmit} categories={[cat(), qaCat]} />)
    await page.getByLabelText(/Title/).fill('hello discussion')
    await page.getByRole('button', { name: 'Create discussion' }).click()
    expect(onSubmit).not.toHaveBeenCalled()
  })
})
