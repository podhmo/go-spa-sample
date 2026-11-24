import { render } from 'vitest-browser-react'
import { page } from 'vitest/browser'
import { useReducer } from 'react'
import { describe, expect, it } from 'vitest'

function Counter() {
  const [count, dispatch] = useReducer((c: number) => c + 1, 0)
  return (
    <div>
      <p>count: {count}</p>
      <button onClick={dispatch}>increment</button>
    </div>
  )
}

describe('browser mode smoke', () => {
  it('clicks a button in a real browser', async () => {
    render(<Counter />)
    await page.getByRole('button', { name: 'increment' }).click()
    await expect.element(page.getByText('count: 1')).toBeInTheDocument()
  })
})
