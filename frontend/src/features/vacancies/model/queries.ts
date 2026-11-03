import { queryOptions } from '@tanstack/react-query'

import { vacanciesApi } from '@/features/vacancies/api/vacanciesApi'
import type { components } from '@/shared/api'

type VacancyStatus = components['schemas']['VacancyStatus']

export const vacancyKeys = {
  all: ['vacancies'] as const,
  list: (status?: VacancyStatus) => [...vacancyKeys.all, 'list', status ?? 'all'] as const,
  detail: (vacancyId: string) => [...vacancyKeys.all, 'detail', vacancyId] as const,
}

export function vacancyListQuery(status?: VacancyStatus) {
  return queryOptions({
    queryKey: vacancyKeys.list(status),
    queryFn: () => vacanciesApi.list(status),
  })
}

export function vacancyDetailQuery(vacancyId: string) {
  return queryOptions({
    queryKey: vacancyKeys.detail(vacancyId),
    queryFn: () => vacanciesApi.get(vacancyId),
    enabled: Boolean(vacancyId),
  })
}
