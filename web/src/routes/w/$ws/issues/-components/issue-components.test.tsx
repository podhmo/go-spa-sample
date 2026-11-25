import { page } from 'vitest/browser'
import { describe, expect, it, vi } from 'vitest'
import { renderWithRouter } from '../../../../../test-utils'
import { IssueForm } from './IssueForm'
import { IssueList } from './IssueList'

describe('component: IssueList', () => {
  it('renders issues and an empty state', async () => {
    renderWithRouter(<IssueList issues={[]} ws="default" />)
    await expect.element(page.getByText('No issues here yet.')).toBeInTheDocument()
  })
})

describe('component: IssueForm', () => {
  it('submits entered values', async () => {
    const onSubmit = vi.fn()
    renderWithRouter(<IssueForm onSubmit={onSubmit} />)
    await page.getByLabelText(/Title/).fill('hello from test')
    await page.getByRole('button', { name: 'Create issue' }).click()
    expect(onSubmit).toHaveBeenCalledWith({ title: 'hello from test', body: '', label_ids: [] })
  })
})
