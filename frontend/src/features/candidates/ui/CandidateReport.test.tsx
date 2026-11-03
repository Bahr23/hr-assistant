import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it } from 'vitest'
import type { components } from '@/shared/api'
import { CandidateReport } from './CandidateReport'

afterEach(cleanup)
type Report = components['schemas']['CandidateReport']
const report: Report = {
  status: 'succeeded', version: 1, result_version: 1, overall_score: 7.3, scored_criteria: 1, total_criteria: 2,
  error_code: null, model: 'model', prompt_version: 'v1', schema_version: 'v1',
  result: {
    summary: 'Есть пример работы с клиентом.',
    criterion_scores: [
      { criterion_id: 'one', name: 'Работа с клиентами', status: 'scored', score: 7.3, summary: 'Выяснил потребность.', evidence: [{ question_number: 1, quote: 'Я уточнил причину' }], missing_information: [] },
      { criterion_id: 'two', name: 'Командная работа', status: 'insufficient_data', score: null, summary: 'Нет примера.', evidence: [], missing_information: ['Пример взаимодействия'] },
    ], strengths: [], concerns: [], missing_information: ['Уточнить общую информацию'], follow_up_questions: [{ question: 'Чем закончился разговор?', reason: 'Нужно уточнить результат.' }],
    analysis_completeness: { status: 'partial', processed_answers: 1, total_answers: 2, issues: ['Вопрос 2: ответ недоступен.'] },
  },
}
function show(value: Report) {
  return render(<CandidateReport report={value} />)
}
it('shows compact criterion descriptions and expands quotes and missing information independently', async () => {
  show(report)
  expect(screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent)).toEqual([
    'Краткий вывод', 'Оценки по критериям', 'Сильные стороны', 'Сомнения и риски', 'Вопросы для следующего этапа',
  ])
  expect(screen.queryByText('Недостающая информация')).not.toBeInTheDocument()
  expect(screen.queryByText('Уточнить общую информацию')).not.toBeInTheDocument()
  expect(screen.getByText('Оценено 1 из 2 критериев')).toBeVisible()
  expect(screen.getByText('Недостаточно данных')).toBeVisible()
  expect(screen.getByText((_, element) => element?.tagName === 'SPAN' && element.textContent === '7,3 / 10')).toBeVisible()
  expect(screen.getByText('Анализ выполнен по неполным данным')).toBeVisible()
  expect(screen.getByText('Выяснил потребность.')).toBeVisible()
  expect(screen.getByText('Я уточнил причину')).not.toBeVisible()
  expect(screen.getByText('Пример взаимодействия')).not.toBeVisible()
  const details = screen.getByLabelText('Обоснование оценки: Работа с клиентами')
  await userEvent.click(details)
  expect(screen.getByText('Я уточнил причину')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Ответ на вопрос 1' })).toHaveAttribute('href', '#answer-1')
  expect(screen.getByText('Пример взаимодействия')).not.toBeVisible()
  await userEvent.click(screen.getByLabelText('Обоснование оценки: Командная работа'))
  expect(screen.getByText('Пример взаимодействия')).toBeVisible()
  await userEvent.click(details)
  expect(screen.getByText('Я уточнил причину')).not.toBeVisible()
  expect(screen.getByText('Пример взаимодействия')).toBeVisible()
  expect(screen.getByText('Чем закончился разговор?')).toBeVisible()
})
it('retains previous success while a new version is pending or failed', () => {
  const pending = show({ ...report, status: 'pending', version: 2 })
  expect(screen.getByText(report.result!.summary)).toBeVisible()
  expect(screen.getByRole('status')).toHaveTextContent('Готовим отчёт.')
  pending.unmount()
  show({ ...report, status: 'permanent_failed', version: 2 })
  expect(screen.getByText(/Показан предыдущий успешный отчёт/)).toBeVisible()
  expect(screen.getByRole('alert')).toHaveTextContent('Не удалось собрать новую версию отчёта.')
})
it('does not show numbers without criteria', () => {
  show({ ...report, total_criteria: 0, scored_criteria: 0, overall_score: null, result: { ...report.result!, criterion_scores: [] } })
  expect(screen.queryByText('Общий балл')).not.toBeInTheDocument()
  expect(screen.queryByText(/Оценено/)).not.toBeInTheDocument()
  expect(screen.getByText(report.result!.summary)).toBeVisible()
})

it('distinguishes a zero score from unavailable data and describes coverage', () => {
  const zero = show({ ...report, overall_score: 0 })
  const score = within(screen.getByLabelText('Общий балл кандидата'))
  expect(score.getByText('0,0')).toBeVisible()
  expect(score.getByRole('meter', { name: 'Покрытие критериев' })).toHaveAttribute('aria-valuenow', '1')
  expect(score.getByRole('meter')).toHaveAttribute('aria-valuemax', '2')
  zero.unmount()
  show({ ...report, overall_score: null, scored_criteria: 0 })
  const missing = within(screen.getByLabelText('Общий балл кандидата'))
  expect(missing.getByText('Недостаточно данных')).toBeVisible()
  expect(missing.queryByText('0,0')).not.toBeInTheDocument()
  expect(missing.getByRole('meter')).toHaveAttribute('aria-valuenow', '0')
})
