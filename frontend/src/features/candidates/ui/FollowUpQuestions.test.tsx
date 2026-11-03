import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { FollowUpQuestions } from './FollowUpQuestions'

const questions = [
  { question: 'Как проверяли результат?', reason: 'Уточнить измеримый эффект.' },
  { question: 'Что сделали лично?', reason: 'Отделить личный вклад от командного.' },
]

afterEach(() => { cleanup(); vi.restoreAllMocks() })

it('copies the ordered interview questions with their reasons', async () => {
  const user = userEvent.setup()
  const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  render(<FollowUpQuestions questions={questions} />)
  await user.click(screen.getByRole('button', { name: 'Скопировать вопросы' }))
  expect(write).toHaveBeenCalledWith('1. Как проверяли результат?\nЗачем уточнить: Уточнить измеримый эффект.\n\n2. Что сделали лично?\nЗачем уточнить: Отделить личный вклад от командного.')
  expect(screen.getByRole('status')).toHaveTextContent('Вопросы скопированы')
})

it('keeps questions available when copying fails and allows another attempt', async () => {
  const user = userEvent.setup()
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValueOnce(new Error('Permission denied')).mockResolvedValueOnce()
  render(<FollowUpQuestions questions={questions} />)
  const button = screen.getByRole('button', { name: 'Скопировать вопросы' })
  await user.click(button)
  expect(screen.getByRole('alert')).toHaveTextContent('скопируйте вручную')
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
  expect(screen.getByText(questions[0]!.question)).toBeVisible()
  await user.click(button)
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent('Вопросы скопированы')
})
