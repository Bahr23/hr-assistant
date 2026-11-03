import { api, apiFailure, type components } from '@/shared/api'

type VacancyStatus = components['schemas']['VacancyStatus']
type VacancyPatch = components['schemas']['VacancyPatch']
type VacancyCreate = components['schemas']['VacancyCreate']
type AnswerType = components['schemas']['QuestionAnswerType']

async function list(status?: VacancyStatus, limit = 20, offset = 0) {
  const { data, error, response } = await api.GET('/vacancies', {
    params: { query: { limit, offset, status } },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function create(body: VacancyCreate) {
  const { data, error, response } = await api.POST('/vacancies', {
    body,
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function get(vacancyId: string) {
  const { data, error, response } = await api.GET('/vacancies/{vacancyId}', {
    params: { path: { vacancyId } },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function update(vacancyId: string, body: VacancyPatch) {
  const { data, error, response } = await api.PATCH('/vacancies/{vacancyId}', {
    params: { path: { vacancyId } },
    body,
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function addQuestion(vacancyId: string, text: string, answerType: AnswerType, evaluationGuide: string | null) {
  const { data, error, response } = await api.POST('/vacancies/{vacancyId}/questions', {
    params: { path: { vacancyId } },
    body: { text, answer_type: answerType, evaluationGuide },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function deleteQuestion(vacancyId: string, questionId: string) {
  const { error, response } = await api.DELETE('/vacancies/{vacancyId}/questions/{questionId}', {
    params: { path: { vacancyId, questionId } },
  })
  if (!response.ok) throw apiFailure(error, response)
}

async function updateQuestion(vacancyId: string, questionId: string, text: string, answerType: AnswerType, evaluationGuide: string | null) {
  const { data, error, response } = await api.PATCH('/vacancies/{vacancyId}/questions/{questionId}', {
    params: { path: { vacancyId, questionId } },
    body: { text, answer_type: answerType, evaluationGuide },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function reorderQuestions(vacancyId: string, questionIds: string[]) {
  const { data, error, response } = await api.PUT('/vacancies/{vacancyId}/questions/order', {
    params: { path: { vacancyId } },
    body: { question_ids: questionIds },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function action(vacancyId: string, kind: 'archive' | 'restore' | 'rotate') {
  const path = kind === 'rotate' ? '/vacancies/{vacancyId}/link/rotate' : `/vacancies/{vacancyId}/${kind}`
  const { data, error, response } = await api.POST(path as '/vacancies/{vacancyId}/archive', {
    params: { path: { vacancyId } },
  })
  if (!data) throw apiFailure(error, response)
  return data
}

async function templates() {
 const {data,error,response}=await api.GET("/evaluation-criteria/templates")
 if (!data) throw apiFailure(error,response)
 return data
}

export const vacanciesApi = { templates, action, addQuestion, create, deleteQuestion, get, list, reorderQuestions, update, updateQuestion }
