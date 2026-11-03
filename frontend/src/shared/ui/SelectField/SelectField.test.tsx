import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { SelectField } from './SelectField'

const options = [
  { value: 'text', label: 'Текстовый ответ' },
  { value: 'voice', label: 'Голосовое сообщение' },
] as const

function ExampleSelect() {
  const [value, setValue] = useState<'text' | 'voice'>('text')
  return (
    <SelectField
      label="Формат ответа"
      value={value}
      options={options}
      onValueChange={setValue}
    />
  )
}

describe('SelectField', () => {
  it('uses the project dropdown instead of a native select', async () => {
    const user = userEvent.setup()
    const { container } = render(<ExampleSelect />)
    const trigger = screen.getByRole('combobox', { name: 'Формат ответа' })

    expect(container.querySelector('select')).not.toBeInTheDocument()
    expect(trigger).toHaveTextContent('Текстовый ответ')

    await user.click(trigger)
    await user.click(screen.getByRole('option', { name: 'Голосовое сообщение' }))

    expect(trigger).toHaveTextContent('Голосовое сообщение')
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  it('supports keyboard selection', async () => {
    const user = userEvent.setup()
    render(<ExampleSelect />)
    const trigger = screen.getByRole('combobox', { name: 'Формат ответа' })

    trigger.focus()
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}')

    expect(trigger).toHaveTextContent('Голосовое сообщение')
  })

  it('opens above when there is not enough space below', async () => {
    const user = userEvent.setup()
    render(<ExampleSelect />)
    const trigger = screen.getByRole('combobox', { name: 'Формат ответа' })
    const field = trigger.parentElement
    if (!field) throw new Error('Select field root is missing')

    vi.spyOn(field, 'getBoundingClientRect').mockReturnValue({
      bottom: 750,
      height: 52,
      left: 0,
      right: 400,
      top: 698,
      width: 400,
      x: 0,
      y: 698,
      toJSON: () => ({}),
    })

    await user.click(trigger)

    await waitFor(() => expect(screen.getByRole('listbox')).toHaveAttribute('data-placement', 'above'))
  })
})
