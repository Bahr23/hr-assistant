import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { Button } from './Button'

describe('Button', () => {
  it('calls the handler once', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Сохранить</Button>)

    await user.click(screen.getByRole('button', { name: 'Сохранить' }))

    expect(onClick).toHaveBeenCalledOnce()
  })

  it('blocks interaction while loading', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<Button loading onClick={onClick}>Сохранить</Button>)

    const button = screen.getByRole('button', { name: 'Сохранить' })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(onClick).not.toHaveBeenCalled()
  })
})
