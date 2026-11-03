import { queryOptions } from '@tanstack/react-query'

import { candidatesApi, type CandidateListParams } from '@/features/candidates/api/candidatesApi'
import { vacanciesApi } from '@/features/vacancies'

export const candidateKeys = {
  all: ['candidates'] as const,
  list: (params: CandidateListParams) => [...candidateKeys.all, 'list', params] as const,
  detail: (sessionId: string) => [...candidateKeys.all, 'detail', sessionId] as const,
}

export function candidateListQuery(params: CandidateListParams) {
  return queryOptions({
    queryKey: candidateKeys.list(params),
    queryFn: () => candidatesApi.list(params),
  })
}

export function candidateDetailQuery(sessionId: string) {
  return queryOptions({
    queryKey: candidateKeys.detail(sessionId),
    queryFn: () => candidatesApi.get(sessionId),
    enabled: Boolean(sessionId),
    refetchInterval: (query) => query.state.data?.report?.status === 'pending' || query.state.data?.answers.some((answer) =>
      answer.evaluation.status === 'pending' || answer.transcription?.status === 'pending',
    ) ? 5000 : false,
  })
}

export const candidateVacancyOptionsQuery = queryOptions({
  queryKey: ['vacancies', 'candidate-filter-options'],
  queryFn: async () => {
    const result = []
    const pageSize = 100
    let offset = 0
    while (true) {
      const page = await vacanciesApi.list(undefined, pageSize, offset)
      result.push(...page.items)
      if (page.items.length < pageSize) return result
      offset += pageSize
    }
  },
})
