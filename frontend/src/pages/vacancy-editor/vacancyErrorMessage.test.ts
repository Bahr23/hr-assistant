import { describe, expect, it } from 'vitest'

import { ApiError } from '@/shared/api'
import { vacancyErrorMessage } from './vacancyErrorMessage'

describe('vacancyErrorMessage', () => {
  it('names the missing publication field', () => {
    const error = new ApiError('is required for publication', {
      code: 'publication_incomplete',
      field: 'title',
    })

    expect(vacancyErrorMessage(error)).toBe('Заполните поле «Название вакансии».')
  })

  it('explains that an interview question is required', () => {
    const error = new ApiError('at least one question is required', {
      code: 'publication_incomplete',
      field: 'questions',
    })

    expect(vacancyErrorMessage(error)).toBe('Добавьте хотя бы один вопрос для интервью.')
  })
})
