import type { ApiErrorBody } from '../types'

export const UNAUTHORIZED_EVENT = 'bengalpos:unauthorized'

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export interface ApiRequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  body?: unknown
  headers?: Record<string, string>
  skipAuthRedirect?: boolean
}

async function toApiError(response: Response): Promise<ApiError> {
  let code = 'error_desconocido'
  let message = 'Ocurrió un error inesperado. Intente de nuevo.'
  try {
    const data = (await response.json()) as ApiErrorBody
    if (data?.error?.code) code = data.error.code
    if (data?.error?.message) message = data.error.message
  } catch {
    // Non-JSON error body: keep the Spanish fallback.
  }
  return new ApiError(response.status, code, message)
}

function notifyUnauthorized(): void {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
  }
}

export async function apiRequest<T>(
  path: string,
  options: ApiRequestOptions = {},
): Promise<T> {
  const { method = 'GET', body, headers = {}, skipAuthRedirect = false } = options
  const requestHeaders: Record<string, string> = {
    Accept: 'application/json',
    ...headers,
  }
  if (body !== undefined) {
    requestHeaders['Content-Type'] = 'application/json'
  }

  let response: Response
  try {
    response = await fetch(path, {
      method,
      credentials: 'include',
      headers: requestHeaders,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError(
      0,
      'error_red',
      'No se pudo conectar con el servidor. Verifique la red e intente de nuevo.',
    )
  }

  if (response.status === 401 && !skipAuthRedirect) {
    notifyUnauthorized()
  }
  if (!response.ok) {
    throw await toApiError(response)
  }
  if (response.status === 204) {
    return undefined as T
  }
  const text = await response.text()
  if (text === '') {
    return undefined as T
  }
  try {
    return JSON.parse(text) as T
  } catch {
    throw new ApiError(
      response.status,
      'respuesta_invalida',
      'El servidor devolvió una respuesta inválida.',
    )
  }
}

export function apiGet<T>(path: string): Promise<T> {
  return apiRequest<T>(path)
}

export function apiPost<T>(
  path: string,
  body: unknown = {},
  headers?: Record<string, string>,
): Promise<T> {
  return apiRequest<T>(path, { method: 'POST', body, headers })
}

export function apiPut<T>(
  path: string,
  body: unknown = {},
  headers?: Record<string, string>,
): Promise<T> {
  return apiRequest<T>(path, { method: 'PUT', body, headers })
}

/** Fetch a print-ready HTML document (the API returns text/html directly). */
export async function apiPrintHtml(path: string): Promise<string> {
  let response: Response
  try {
    response = await fetch(path, {
      method: 'POST',
      credentials: 'include',
      headers: {
        Accept: 'text/html',
        'Content-Type': 'application/json',
      },
      body: '{}',
    })
  } catch {
    throw new ApiError(
      0,
      'error_red',
      'No se pudo conectar con el servidor. Verifique la red e intente de nuevo.',
    )
  }
  if (response.status === 401) {
    notifyUnauthorized()
  }
  if (!response.ok) {
    throw await toApiError(response)
  }
  return response.text()
}

/** Error message safe to show in the UI (Spanish, backend message first). */
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error && error.message) return error.message
  return 'Ocurrió un error inesperado. Intente de nuevo.'
}
