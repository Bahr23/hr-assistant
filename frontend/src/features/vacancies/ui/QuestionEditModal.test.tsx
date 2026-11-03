import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { QuestionEditModal } from './QuestionEditModal'

// jsdom does not implement the native dialog methods.
Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
  configurable: true,
  value(this: HTMLDialogElement) { this.setAttribute('open', '') },
})
Object.defineProperty(HTMLDialogElement.prototype, 'close', {
  configurable: true,
  value(this: HTMLDialogElement) { this.removeAttribute('open') },
})

afterEach(cleanup)

describe('question evaluation guide', () => {
  it.each(['text', 'voice'] as const)('saves a guide-only change for %s and keeps the hint visible', async (answerType) => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<QuestionEditModal initialText="Вопрос" initialAnswerType={answerType} onClose={vi.fn()} onSave={save} />)
    const guide = screen.getByRole('textbox', { name: 'Что искать в ответе' })
    expect(screen.getByRole('button', { name: 'Сохранить' })).toBeDisabled()
    expect(guide).toHaveAttribute('maxlength', '4000')
    await user.type(guide, '  Конкретный пример  ')
    expect(guide).toHaveAccessibleDescription(/Опишите, что должен содержать хороший ответ/)
    expect(screen.getByText(/Поле необязательное/)).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Сохранить' }))
    expect(save).toHaveBeenCalledWith('Вопрос', answerType, 'Конкретный пример')
  })

  it('clears a guide to null', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<QuestionEditModal initialText="Вопрос" initialAnswerType="text" initialEvaluationGuide="Пример" onClose={vi.fn()} onSave={save} />)
    await user.clear(screen.getByRole('textbox', { name: 'Что искать в ответе' }))
    await user.click(screen.getByRole('button', { name: 'Сохранить' }))
    expect(save).toHaveBeenCalledWith('Вопрос', 'text', null)
  })
})
