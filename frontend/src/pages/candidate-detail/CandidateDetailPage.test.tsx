import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { components } from '@/shared/api'
import { candidateKeys, candidatesApi } from '@/features/candidates'
import { CandidateDetailPage } from './CandidateDetailPage'

const date = '2026-09-01T10:00:00Z'
const detail: components['schemas']['CandidateInterviewDetail'] = {
  id: 'session', started_at: date, completed_at: date, duration_seconds: 60,
  vacancy: { id: 'vacancy', title: 'Аналитик' },
  candidate: { telegram_user_id: 1, username: null, first_name: 'Тест', last_name: null, is_premium: false, language_code: null, first_seen_at: date, last_seen_at: date },
  report: {
    status: 'succeeded', version: 1, result_version: 1, overall_score: 7, scored_criteria: 1, total_criteria: 1,
    error_code: null, model: null, prompt_version: null, schema_version: null,
    result: {
      summary: 'Есть пример исследования.',
      criterion_scores: [{ criterion_id: 'criterion', name: 'Анализ', status: 'scored', score: 7, summary: 'Описан подход.', evidence: [{ question_number: 2, quote: 'Проверил гипотезу' }], missing_information: [] }],
      strengths: [], concerns: [], missing_information: [], follow_up_questions: [],
      analysis_completeness: { status: 'complete', processed_answers: 2, total_answers: 2, issues: [] },
    },
  },
  answers: [0, 1].map((position) => ({
    id: `answer-${position}`, position, question: `Расскажите о проекте ${position + 1}`, answer_type: 'text', answered_at: date,
    text: position === 0 ? 'Первый ответ' : 'Проверил гипотезу на данных', transcription: null,
    evaluation: { status: 'skipped', result: null, version: null, resultVersion: null, reason: 'no_guide' },
  })),
}

beforeEach(() => { Element.prototype.scrollIntoView = vi.fn() })
afterEach(() => { cleanup(); window.history.replaceState(null, '', '/'); vi.restoreAllMocks() })

function show(hash = '', value = detail) {
  window.history.replaceState(null, '', `/candidates/session${hash}`)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(candidateKeys.detail('session'), value)
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[{ pathname: '/candidates/session', hash, state: { returnTo: '/candidates?search=test' } }]}>
    <Routes><Route path="/candidates/:sessionId" element={<CandidateDetailPage />} /></Routes>
  </MemoryRouter></QueryClientProvider>)
}

it('keeps answers compact and opens the quoted source, including repeated links', async () => {
  show()
  expect(screen.getByText('Первый ответ')).not.toBeVisible()
  expect(screen.getByText('Проверил гипотезу на данных')).not.toBeVisible()
  await userEvent.click(screen.getByLabelText('Обоснование оценки: Анализ'))
  const source = screen.getByRole('link', { name: 'Ответ на вопрос 2' })
  await userEvent.click(source)
  expect(screen.getByText('Проверил гипотезу на данных')).toBeVisible()
  expect(screen.getByText('Первый ответ')).not.toBeVisible()
  const summary = screen.getByText('Расскажите о проекте 2').closest('summary')!
  expect(summary).toHaveFocus()
  await userEvent.click(summary)
  expect(screen.getByText('Проверил гипотезу на данных')).not.toBeVisible()
  await userEvent.click(source)
  expect(screen.getByText('Проверил гипотезу на данных')).toBeVisible()
  expect(screen.getByRole('link', { name: 'К кандидатам' })).toHaveAttribute('href', '/candidates?search=test')
})

it('opens a direct answer URL and follows subsequent hash navigation', () => {
  show('#answer-2')
  expect(screen.getByText('Проверил гипотезу на данных')).toBeVisible()
  expect(screen.getByText('Первый ответ')).not.toBeVisible()
  act(() => {
    window.history.replaceState(null, '', '#answer-1')
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  })
  expect(screen.getByText('Первый ответ')).toBeVisible()
})

it('returns from the answer to its original quote and restores the disclosure and focus', async () => {
  show()
  const criterion = screen.getByLabelText('Обоснование оценки: Анализ')
  await userEvent.click(criterion)
  const source = screen.getByRole('link', { name: 'Ответ на вопрос 2' })
  await userEvent.click(source)
  const returnLink = screen.getByRole('link', { name: 'Вернуться к цитате' })
  expect(returnLink).toHaveAttribute('href', `#${source.id}`)
  await userEvent.click(criterion)
  expect(source).not.toBeVisible()
  await userEvent.click(returnLink)
  expect(source).toBeVisible()
  expect(source).toHaveFocus()
})

it('links only to available sections while the first report is pending', () => {
  show('', { ...detail, report: { ...detail.report, status: 'pending', result: null } })
  const navigation = within(screen.getByRole('navigation', { name: 'Разделы отчёта' }))
  expect(navigation.getAllByRole('link').map((link) => link.getAttribute('href'))).toEqual(['#report-summary', '#interview-answers'])
  expect(screen.getByRole('status')).toHaveTextContent('Готовим отчёт')
  expect(screen.getByText('Расскажите о проекте 1')).toBeVisible()
})

it('blocks repeated rebuilds, keeps the previous report and polls the accepted version', async () => {
  let accept!: (value: { version: number }) => void
  const request = new Promise<{ version: number }>((resolve) => { accept = resolve })
  const rebuild = vi.spyOn(candidatesApi, 'rebuildReport').mockReturnValue(request)
  const get = vi.spyOn(candidatesApi, 'get').mockResolvedValue({ ...detail, report: { ...detail.report, status: 'pending', version: 3 } })
  show('', { ...detail, report: { ...detail.report, status: 'permanent_failed', version: 2 } })
  await userEvent.click(screen.getByRole('button', { name: 'Повторить сборку' }))
  expect(rebuild).toHaveBeenCalledExactlyOnceWith('session', 2)
  expect(screen.queryByRole('button', { name: 'Повторить сборку' })).not.toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent('Готовим отчёт')
  expect(screen.getByText(detail.report.result!.summary)).toBeVisible()
  await act(async () => { accept({ version: 3 }); await request })
  await waitFor(() => expect(get).toHaveBeenCalledWith('session'))
  expect(screen.getByRole('status')).toHaveTextContent('Предыдущий результат остаётся доступен')
  expect(screen.getByText('Показан предыдущий успешный отчёт — версия 1.')).toBeVisible()
})

it('allows retrying a rejected rebuild and preserves data if the subsequent refresh fails', async () => {
  const rebuild = vi.spyOn(candidatesApi, 'rebuildReport').mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ version: 3 })
  vi.spyOn(candidatesApi, 'get').mockRejectedValue(new Error('offline'))
  show('', { ...detail, report: { ...detail.report, status: 'permanent_failed', version: 2 } })
  await userEvent.click(screen.getByRole('button', { name: 'Повторить сборку' }))
  expect(await screen.findByText('Не удалось запустить сборку. Попробуйте ещё раз.')).toBeVisible()
  expect(screen.getByText(detail.report.result!.summary)).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Повторить сборку' }))
  expect(await screen.findByText('Не удалось обновить данные. Показан последний загруженный отчёт.')).toBeVisible()
  expect(screen.getByText(detail.report.result!.summary)).toBeVisible()
  expect(screen.getByRole('button', { name: 'Обновить данные' })).toBeEnabled()
  expect(rebuild).toHaveBeenCalledTimes(2)
})
