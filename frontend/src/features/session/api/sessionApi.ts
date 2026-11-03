import { apiFailure, publicApi, refreshSession, setAccessToken, type components } from '@/shared/api'

type LoginInput = components['schemas']['LoginRequest']
type RegisterInput = components['schemas']['RegisterRequest']

async function login(body: LoginInput) {
  const { data, error, response } = await publicApi.POST('/auth/login', { body })
  if (!data) throw apiFailure(error, response)
  setAccessToken(data.access_token)
  return data
}

async function register(body: RegisterInput) {
  const { data, error, response } = await publicApi.POST('/auth/register', { body })
  if (!data) throw apiFailure(error, response)
  setAccessToken(data.access_token)
  return data
}

async function logout() {
  const { error, response } = await publicApi.POST('/auth/logout')
  setAccessToken(null)
  if (!response.ok) throw apiFailure(error, response)
}

export const sessionApi = { login, logout, refresh: refreshSession, register }
