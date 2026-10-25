import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { App } from '../App'
import {
  htmlResponse,
  jsonResponse,
  mockApi,
  usuarioFixture,
  type MockRoute,
} from '../test/mockApi'
import type { Comanda, Sale, SaleCompletion, SaleCompletionView } from '../types'

function saleFixture(): Sale {
  return {
    id: 77,
    estado: 'completada',
    usuario_id: usuarioFixture.id,
    total_bs: 123456,
    descuento_bs: 0,
    items: [],
    partes: [],
  }
}

function comandaFixture(): Comanda {
  return {
    id: 9,
    venta_id: 77,
    area_preparacion_id: 2,
    area_nombre: 'Cocina',
    orden_impresion: 1,
    estado: 'pendiente',
    impresa: false,
    copia: false,
    items: [],
  }
}

function completion(): SaleCompletion {
  return {
    venta: saleFixture(),
    stock_negativo: false,
    comandas: [comandaFixture()],
    sin_area: [],
  }
}

const view: SaleCompletionView = { completion: completion(), cambioTotal: 76544 }

function renderSuccess() {
  return render(
    <MemoryRouter
      initialEntries={[
        { pathname: '/venta/77/completada', state: view },
      ]}
    >
      <App />
    </MemoryRouter>,
  )
}

function baseRoutes(overrides: MockRoute[] = []): MockRoute[] {
  return [
    {
      method: 'GET',
      path: '/api/auth/me',
      handler: () => jsonResponse({ usuario: usuarioFixture }),
    },
    {
      method: 'GET',
      path: '/api/ventas/77/comandas',
      handler: () => jsonResponse({ comandas: [comandaFixture()] }),
    },
    ...overrides,
  ]
}

function fakePopup() {
  const document = { open: vi.fn(), write: vi.fn(), close: vi.fn() }
  return {
    document,
    focus: vi.fn(),
    print: vi.fn(),
    close: vi.fn(),
  } as unknown as Window & {
    document: { open: ReturnType<typeof vi.fn>; write: ReturnType<typeof vi.fn>; close: ReturnType<typeof vi.fn> }
    print: ReturnType<typeof vi.fn>
  }
}

describe('pantalla de venta completada e impresión', () => {
  it('muestra total, cambio y estado de la venta', async () => {
    mockApi(baseRoutes())
    renderSuccess()

    expect(await screen.findByRole('heading', { name: 'Venta completada' })).toBeInTheDocument()
    expect(screen.getByText('Venta #77')).toBeInTheDocument()
    expect(screen.getByText('Bs 1.234,56')).toBeInTheDocument()
    expect(screen.getByText('Bs 765,44')).toBeInTheDocument()
  })

  it('imprime el recibo obteniendo el HTML y llamando a window.print', async () => {
    const user = userEvent.setup()
    const popup = fakePopup()
    vi.spyOn(window, 'open').mockReturnValue(popup)
    const calls = mockApi(
      baseRoutes([
        {
          method: 'POST',
          path: '/api/ventas/77/recibo/imprimir?formato=html',
          handler: () => htmlResponse('<html><body>RECIBO</body></html>'),
        },
      ]),
    )
    renderSuccess()

    await screen.findByRole('heading', { name: 'Venta completada' })
    await user.click(screen.getByRole('button', { name: 'Imprimir recibo' }))

    await waitFor(() => expect(popup.print).toHaveBeenCalledTimes(1))
    expect(popup.document.write).toHaveBeenCalledWith('<html><body>RECIBO</body></html>')
    const call = calls.find((item) => item.path.includes('/recibo/imprimir'))
    expect(call).toBeDefined()
    expect(call?.method).toBe('POST')
    expect(call?.init.credentials).toBe('include')
  })

  it('imprime cada comanda por su endpoint HTML', async () => {
    const user = userEvent.setup()
    const popup = fakePopup()
    vi.spyOn(window, 'open').mockReturnValue(popup)
    const calls = mockApi(
      baseRoutes([
        {
          method: 'POST',
          path: '/api/comandas/9/imprimir?formato=html',
          handler: () => htmlResponse('<html><body>COCINA</body></html>'),
        },
      ]),
    )
    renderSuccess()

    await screen.findByRole('heading', { name: 'Venta completada' })
    await user.click(screen.getByRole('button', { name: 'Imprimir comandas' }))

    await waitFor(() => expect(popup.print).toHaveBeenCalled())
    expect(popup.document.write).toHaveBeenCalledWith('<html><body>COCINA</body></html>')
    expect(calls.some((item) => item.path.includes('/api/comandas/9/imprimir'))).toBe(true)
  })

  it('informa si el navegador bloquea la ventana de impresión y permite reintentar', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'open').mockReturnValue(null)
    mockApi(baseRoutes())
    renderSuccess()

    await screen.findByRole('heading', { name: 'Venta completada' })
    await user.click(screen.getByRole('button', { name: 'Imprimir recibo' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/bloqueó la ventana de impresión/)
    expect(screen.getByRole('button', { name: 'Imprimir recibo' })).toBeEnabled()
  })
})
