import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import { vacanciesApi } from '../api/vacanciesApi'
import { criteriaValid, type Criterion } from '../model/criteria'
import { CriteriaEditor } from './CriteriaEditor'

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) { this.setAttribute('open', '') }
  HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) { this.removeAttribute('open') }
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })
const templates = [{ id: 't1', title: 'Опыт', evaluationGuide: 'Нужен пример' }, { id: 't2', title: 'Команда', evaluationGuide: 'Работа вместе' }]
function show(initial: Criterion[] = []) {
  vi.spyOn(vacanciesApi, 'templates').mockResolvedValue({ version: 'v1', items: templates })
  function Harness() {
    const [value, setValue] = useState(initial)
    return <><CriteriaEditor value={value} onChange={setValue} /><output data-testid="value">{JSON.stringify(value)}</output></>
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><Harness /></QueryClientProvider>)
}
it('copies selected templates, allows editing, reorder and removal without changing catalog', async () => {
  show()
  expect(screen.getByText(/Без критериев/)).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Выбрать из каталога' }))
  const modal = await screen.findByRole('dialog')
  await userEvent.click(await within(modal).findByRole('checkbox', { name: /Опыт/ }))
  await userEvent.click(within(modal).getByRole('checkbox', { name: /Команда/ }))
  await userEvent.click(within(modal).getByRole('button', { name: 'Добавить выбранные (2)' }))
  const title = screen.getAllByRole('textbox', { name: 'Название' })[0]!
  await userEvent.clear(title)
  await userEvent.type(title, 'Команда')
  expect(screen.getAllByText(/Критерий с таким названием уже добавлен/)).toHaveLength(2)
  expect(templates[0]!.title).toBe('Опыт')
  await userEvent.click(screen.getByRole('button', { name: 'Поднять критерий 2' }))
  let values = JSON.parse(screen.getByTestId('value').textContent) as Criterion[]
  expect(values[0]!.sourceTemplateId).toBe('t2')
  expect(values[1]!.position).toBe(1)
  expect(values[1]!.id).not.toBe('t1')
  await userEvent.click(screen.getByRole('button', { name: 'Удалить критерий 1' }))
  values = JSON.parse(screen.getByTestId('value').textContent) as Criterion[]
  expect(values).toHaveLength(1)
  expect(values[0]!.position).toBe(0)
})
it('creates a custom criterion and requires both fields', async () => {
  show()
  await userEvent.click(screen.getByRole('button', { name: 'Свой критерий' }))
  let values = JSON.parse(screen.getByTestId('value').textContent) as Criterion[]
  expect(criteriaValid(values)).toBe(false)
  await userEvent.type(screen.getByRole('textbox', { name: 'Название' }), 'Знания')
  await userEvent.type(screen.getByRole('textbox', { name: 'Как оценивать' }), 'Применение на практике')
  values = JSON.parse(screen.getByTestId('value').textContent) as Criterion[]
  expect(criteriaValid(values)).toBe(true)
  expect(values[0]!.sourceTemplateId).toBeNull()
})
it('enforces the twenty criterion limit', () => {
  show(Array.from({ length: 20 }, (_, position) => ({ id: `c${position}`, title: `Критерий ${position}`, evaluationGuide: 'Пример', position, sourceTemplateId: null })))
  expect(screen.getByRole('button', { name: 'Свой критерий' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Выбрать из каталога' })).toBeDisabled()
})
