import { vi } from 'vitest'

export interface ApiCall {
  path: string
  method: string
  init: RequestInit
}

export interface MockRoute {
  method?: string
  path: string | RegExp
  handler: (init: RequestInit, path: string) => Response | Promise<Response>
}

export function jsonResponse(body: unknown, status = 200): Response {
  const text = JSON.stringify(body)
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => JSON.parse(text),
    text: async () => text,
  } as unknown as Response
}

export function emptyResponse(status = 204): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => {
      throw new Error('sin cuerpo')
    },
    text: async () => '',
  } as unknown as Response
}

export function htmlResponse(html: string): Response {
  return {
    ok: true,
    status: 200,
    json: async () => {
      throw new Error('sin cuerpo JSON')
    },
    text: async () => html,
  } as unknown as Response
}

function pathMatches(pattern: string | RegExp, path: string): boolean {
  if (typeof pattern === 'string') return pattern === path
  return pattern.test(path)
}

export function mockApi(routes: MockRoute[]): ApiCall[] {
  const calls: ApiCall[] = []
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const path = typeof input === 'string' ? input : input.toString()
    const method = (init.method ?? 'GET').toUpperCase()
    calls.push({ path, method, init })
    const route = routes.find(
      (candidate) =>
        pathMatches(candidate.path, path) &&
        (!candidate.method || candidate.method.toUpperCase() === method),
    )
    if (!route) {
      throw new Error(`Fetch inesperado: ${method} ${path}`)
    }
    return route.handler(init, path)
  })
  vi.stubGlobal('fetch', fetchMock)
  return calls
}

export const usuarioFixture = {
  id: 7,
  nombre_completo: 'Ana Pérez',
  login: 'ana',
  rol: 'encargado',
}

export const productosFixture = [
  { id: 1, codigo: 'P001', nombre: 'Café con leche', precio_venta_bs: 123456, activo: true },
  { id: 2, codigo: 'P002', nombre: 'Pan de queso', precio_venta_bs: 5000, activo: true },
]

export const depositosFixture = [{ id: 3, nombre: 'Depósito principal', activo: true }]

export function baseRoutes(overrides: MockRoute[] = []): MockRoute[] {
  return [
    {
      method: 'GET',
      path: '/api/auth/me',
      handler: () => jsonResponse({ usuario: usuarioFixture }),
    },
    {
      method: 'GET',
      path: '/api/productos',
      handler: () => jsonResponse({ productos: productosFixture }),
    },
    {
      method: 'GET',
      path: '/api/depositos',
      handler: () => jsonResponse({ depositos: depositosFixture }),
    },
    {
      method: 'GET',
      path: '/api/caja/abierta',
      handler: () =>
        jsonResponse({ caja: { id: 1, estado: 'abierta', monto_inicial_bs: 0 } }),
    },
    ...overrides,
  ]
}
