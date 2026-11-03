import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { components } from '@/shared/api'
import { candidatesApi } from '../api/candidatesApi'
import { AnswerEvaluation } from './AnswerEvaluation'

afterEach(() => { cleanup(); vi.restoreAllMocks() })

type Evaluation = components['schemas']['AnswerEvaluation']
const result: NonNullable<Evaluation['result']> = {
  summary: 'Кандидат уточнил причину, итог не указан.',
  checks: [
    { criterion: 'Выяснение причины', type: 'expected', status: 'found', evidence: ['Я спросил клиента'], explanation: 'Уточнил причину.' },
    { criterion: 'Результат разговора', type: 'expected', status: 'not_found', evidence: [], explanation: 'Итог не указан.' },
    { criterion: 'Обещание без согласования', type: 'unwanted', status: 'found', evidence: ['Я пообещал'], explanation: 'Описал обещание.' },
  ],
  follow_up_questions: ['Чем закончился разговор?'],
}

function show(evaluation: Evaluation) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><AnswerEvaluation sessionId="session" answerId="answer" evaluation={evaluation} /></QueryClientProvider>)
}

describe('answer evaluation', () => {
  it('opens check groups independently while keeping the summary and counts visible', async () => {
    show({ status: 'succeeded', result, version: 1, resultVersion: 1, reason: null })
    expect(screen.getByText(result.summary)).toBeVisible()
    expect(screen.getByText('Я спросил клиента')).not.toBeVisible()
    expect(screen.getByLabelText('Не подтверждено или требует уточнения')).toHaveAccessibleDescription('Пунктов: 1')
    await userEvent.click(screen.getByLabelText('Найденные ожидаемые элементы'))
    expect(screen.getByText('Я спросил клиента')).toBeVisible()
    expect(screen.getByText('Я спросил клиента').tagName).toBe('BLOCKQUOTE')
    expect(screen.getByRole('heading', { name: 'Найденные нежелательные признаки' })).toBeVisible()
    expect(screen.getByText('Я пообещал')).not.toBeVisible()
    await userEvent.click(screen.getByLabelText('Не подтверждено или требует уточнения'))
    expect(screen.getByText('Итог не указан.')).toBeVisible()
    await userEvent.click(screen.getByLabelText('Найденные ожидаемые элементы'))
    expect(screen.getByText('Я спросил клиента')).not.toBeVisible()
    expect(screen.getByText('Итог не указан.')).toBeVisible()
    expect(screen.getByText('Чем закончился разговор?')).not.toBeVisible()
    await userEvent.click(screen.getByLabelText('Уточняющие вопросы'))
    expect(screen.getByText('Чем закончился разговор?')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Повторить проверку' })).not.toBeInTheDocument()
  })

  it('retries the failed version and retains the previous success', async () => {
    const retry = vi.spyOn(candidatesApi, 'retryEvaluation').mockResolvedValue({ version: 3 })
    show({ status: 'permanent_failed', result, version: 2, resultVersion: 1, reason: null })
    expect(screen.getByText('Не удалось выполнить проверку')).toBeVisible()
    expect(screen.getByText(result.summary)).toBeVisible()
    expect(screen.getByText('Показан предыдущий успешный результат.')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Повторить проверку' }))
    await waitFor(() => expect(retry).toHaveBeenCalledWith('session', 'answer', 2))
  })

  it('distinguishes pending analysis from a failed transcription', () => {
    const { unmount } = show({ status: 'pending', result: null, version: 1, resultVersion: null, reason: null })
    expect(screen.getByRole('status')).toHaveTextContent('Проверяем ответ')
    unmount()
    show({ status: 'skipped', result: null, version: null, resultVersion: null, reason: 'transcription_failed' })
    expect(screen.getByText(/не удалось получить текст голосового ответа/)).toBeVisible()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('explains exhausted automatic attempts without assigning a low score', () => {
    show({ status: 'permanent_failed', result: null, version: 1, resultVersion: null, reason: null, attempts: 3, errorCode: 'invalid_result' })
    expect(screen.getByText('Результат AI не прошёл проверку формата или дословности цитат.')).toBeVisible()
    expect(screen.getByText(/Выполнено попыток: 3/)).toHaveTextContent('Ошибка обработки не снижает оценку кандидата.')
    expect(screen.getByRole('button', { name: 'Повторить проверку' })).toBeEnabled()
  })
})
