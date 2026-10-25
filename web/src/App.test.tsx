import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import {
  baseRoutes,
  emptyResponse,
  jsonResponse,
  mockApi,
  type MockRoute,
  usuarioFixture,
} from './test/mockApi'

function renderApp(path = '/venta') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
}

const unauthorizedRoute: MockRoute = {
  method: 'GET',
  path: '/api/auth/me',
  handler: () =>
    jsonResponse(
      { error: { code: 'no_autenticado', message: 'Debe iniciar sesión' } },
      401,
    ),
}

describe('autenticación y rutas protegidas', () => {
  it('redirige a /login cuando no hay sesión', async () => {
    mockApi([unauthorizedRoute])
    renderApp('/venta')

    expect(await screen.findByLabelText('Usuario')).toBeInTheDocument()
    expect(screen.getByLabelText('Contraseña')).toBeInTheDocument()
  })

  it('muestra "Credenciales inválidas" y no navega con un login inválido', async () => {
    const user = userEvent.setup()
    mockApi([
      unauthorizedRoute,
      {
        method: 'POST',
        path: '/api/auth/login',
        handler: () =>
          jsonResponse(
            { error: { code: 'credenciales_invalidas', message: 'Credenciales inválidas' } },
            401,
          ),
      },
    ])
    renderApp('/venta')

    await user.type(await screen.findByLabelText('Usuario'), 'ana')
    await user.type(screen.getByLabelText('Contraseña'), 'incorrecta')
    await user.click(screen.getByRole('button', { name: 'Ingresar' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Credenciales inválidas')
    expect(screen.getByLabelText('Usuario')).toBeInTheDocument()
  })

  it('inicia sesión, carga la venta y cierra sesión', async () => {
    const user = userEvent.setup()
    const calls = mockApi([
      unauthorizedRoute,
      {
        method: 'POST',
        path: '/api/auth/login',
        handler: () => jsonResponse({ usuario: usuarioFixture }),
      },
      ...baseRoutes().filter((route) => route.path !== '/api/auth/me'),
      {
        method: 'POST',
        path: '/api/auth/logout',
        handler: () => emptyResponse(204),
      },
    ])

    renderApp('/venta')
    await user.type(await screen.findByLabelText('Usuario'), 'ana')
    await user.type(screen.getByLabelText('Contraseña'), 'Clave1234')
    await user.click(screen.getByRole('button', { name: 'Ingresar' }))

    expect(await screen.findByText('Café con leche')).toBeInTheDocument()
    expect(screen.getByText('Ana Pérez')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cerrar sesión' }))

    expect(await screen.findByLabelText('Usuario')).toBeInTheDocument()
    expect(calls.some((call) => call.path === '/api/auth/logout')).toBe(true)
  })

  it('restaura la sesión con /api/auth/me al recargar', async () => {
    const calls = mockApi(baseRoutes())
    renderApp('/venta')

    expect(await screen.findByText('Café con leche')).toBeInTheDocument()
    expect(screen.getByText('Ana Pérez')).toBeInTheDocument()
    expect(calls.some((call) => call.path === '/api/auth/me')).toBe(true)
  })

  it('redirige a /login ante un 401 posterior de cualquier endpoint', async () => {
    mockApi([
      {
        method: 'GET',
        path: '/api/auth/me',
        handler: () => jsonResponse({ usuario: usuarioFixture }),
      },
      {
        method: 'GET',
        path: '/api/productos',
        handler: () =>
          jsonResponse(
            { error: { code: 'no_autenticado', message: 'Debe iniciar sesión' } },
            401,
          ),
      },
      {
        method: 'GET',
        path: '/api/depositos',
        handler: () => jsonResponse({ depositos: [] }),
      },
      {
        method: 'GET',
        path: '/api/caja/abierta',
        handler: () => jsonResponse({ caja: null }),
      },
    ])
    renderApp('/venta')

    await waitFor(() => {
      expect(screen.getByLabelText('Usuario')).toBeInTheDocument()
    })
  })
})
