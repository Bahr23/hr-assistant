import createClient from 'openapi-fetch'

import type { components, paths } from './schema.generated'

const apiBaseUrl = (import.meta.env.VITE_API_URL ?? 'http://localhost:8080/api/v1').replace(/\/$/, '')

type AuthResponse = components['schemas']['AuthResponse']
type ErrorPayload = components['schemas']['Error']

let accessToken: string | null = null
let refreshInFlight: Promise<AuthResponse> | null = null

export class ApiError extends Error {
  readonly code: string
  readonly requestId?: string
  readonly field?: string
  readonly status?: number

  constructor(message: string, options: { code?: string; requestId?: string; field?: string; status?: number } = {}) {
    super(message)
    this.name = 'ApiError'
    this.code = options.code ?? 'unexpected_error'
    this.requestId = options.requestId
    this.field = options.field
    this.status = options.status
  }
}

export function setAccessToken(token: string | null) {
  accessToken = token
}

async function readError(response: Response): Promise<ApiError> {
  try {
    const payload = (await response.clone().json()) as ErrorPayload
    const details = payload.error.details as { field?: string } | undefined
    return new ApiError(payload.error.message, {
      code: payload.error.code,
      requestId: payload.error.request_id,
      field: details?.field,
      status: response.status,
    })
  } catch {
    return new ApiError('Не удалось выполнить запрос', { status: response.status })
  }
}

async function rawRefresh(): Promise<AuthResponse> {
  const response = await globalThis.fetch(`${apiBaseUrl}/auth/refresh`, {
    method: 'POST',
    credentials: 'include',
    headers: { Accept: 'application/json' },
  })

  if (!response.ok) {
    setAccessToken(null)
    throw await readError(response)
  }

  const auth = (await response.json()) as AuthResponse
  setAccessToken(auth.access_token)
  return auth
}

export function refreshSession() {
  if (!refreshInFlight) {
    refreshInFlight = rawRefresh().finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

const authenticatedFetch: typeof globalThis.fetch = async (input, init) => {
  const request = new Request(input, { ...init, credentials: 'include' })
  if (accessToken) {
    request.headers.set('Authorization', `Bearer ${accessToken}`)
  }
  const retrySource = request.clone()
  const response = await globalThis.fetch(request)

  if (response.status !== 401 || retrySource.url.endsWith('/auth/refresh')) {
    return response
  }

  try {
    const auth = await refreshSession()
    const retry = new Request(retrySource)
    retry.headers.set('Authorization', `Bearer ${auth.access_token}`)
    return globalThis.fetch(retry)
  } catch {
    return response
  }
}

export const publicApi = createClient<paths>({
  baseUrl: apiBaseUrl,
  credentials: 'include',
})

export const api = createClient<paths>({
  baseUrl: apiBaseUrl,
  credentials: 'include',
  fetch: authenticatedFetch,
})

export function apiFailure(error: unknown, response?: Response): ApiError {
  if (error && typeof error === 'object' && 'error' in error) {
    const payload = error as ErrorPayload
    const details = payload.error.details as { field?: string } | undefined
    return new ApiError(payload.error.message, {
      code: payload.error.code,
      requestId: payload.error.request_id,
      field: details?.field,
      status: response?.status,
    })
  }
  if (error instanceof Error) return new ApiError(error.message, { status: response?.status })
  return new ApiError('Что-то пошло не так', { status: response?.status })
}
