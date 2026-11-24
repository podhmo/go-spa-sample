import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { describe, expect, it } from 'vitest'

function Hello() {
  return <h1>hello spa</h1>
}

describe('smoke', () => {
  it('renders a component', () => {
    render(<Hello />)
    expect(screen.getByText('hello spa')).toBeInTheDocument()
  })
})
