import { api, apiFailure } from '@/shared/api'

export type CandidateListParams = {
  search?: string
  vacancyId?: string
  limit: number
  offset: number
}

async function list(params: CandidateListParams) {
  const { data, error, response } = await api.GET('/candidates', {
    params: {
      query: {
        search: params.search || undefined,
        vacancy_id: params.vacancyId || undefined,
        limit: params.limit,
        offset: params.offset,
      },
    },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function get(sessionId: string) {
  const { data, error, response } = await api.GET('/candidates/{sessionId}', {
    params: { path: { sessionId } },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function retryEvaluation(sessionId: string, answerId: string, version: number) {
  const { data, error, response } = await api.POST('/candidates/{sessionId}/answers/{answerId}/evaluation/retry', {
    params: { path: { sessionId, answerId } },
    body: { version },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function rebuildReport(sessionId: string, version: number) {
 const {data,error,response}=await api.POST("/candidates/{sessionId}/report/rebuild", {params:{path:{sessionId}},body:{version}})
 if (!data) throw apiFailure(error,response)
 return data
}

export const candidatesApi = { rebuildReport, get, list, retryEvaluation }
