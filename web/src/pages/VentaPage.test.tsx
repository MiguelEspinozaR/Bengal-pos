import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { App } from '../App'
import {
  baseRoutes,
  jsonResponse,
  mockApi,
  usuarioFixture,
  type MockRoute,
} from '../test/mockApi'
import type { Comanda, PaymentPart, Sale, SaleCompletion } from '../types'

function renderVenta() {
  return render(
    <MemoryRouter initialEntries={['/venta']}>
      <App />
    </MemoryRouter>,
  )
}

function withRoutes(overrides: MockRoute[]): MockRoute[] {
  const key = (route: MockRoute) => `${(route.method ?? 'GET').toUpperCase()} ${String(route.path)}`
  const overrideKeys = new Set(overrides.map(key))
  return [
    ...overrides,
    ...baseRoutes().filter((route) => !overrideKeys.has(key(route))),
  ]
}

function saleFixture(overrides: Partial<Sale> = {}): Sale {
  return {
    id: 77,
    estado: 'en_proceso',
    usuario_id: usuarioFixture.id,
    total_bs: 123456,
    descuento_bs: 0,
    items: [],
    partes: [],
    ...overrides,
  }
}

function partFixture(overrides: Partial<PaymentPart> = {}): PaymentPart {
  return {
    id: 1,
    venta_id: 77,
    metodo: 'efectivo',
    monto_bs: 100000,
    estado: 'pendiente',
    cambio_bs: 0,
    ...overrides,
  }
}

function completionFixture(overrides: Partial<SaleCompletion> = {}): SaleCompletion {
  const comanda: Comanda = {
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
  return {
    venta: saleFixture({ estado: 'completada' }),
    stock_negativo: false,
    comandas: [comanda],
    sin_area: [],
    ...overrides,
  }
}

async function addCafeAndCheckout(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Agregar Café con leche' }))
  await user.click(screen.getByRole('button', { name: 'Cobrar' }))
  await screen.findByRole('heading', { name: /Cobro de la venta #77/ })
}

describe('pantalla de venta', () => {
  it('busca productos sin acentos, arma el carrito y calcula el total entero', async () => {
    const user = userEvent.setup()
    mockApi(baseRoutes())
    renderVenta()

    const search = await screen.findByLabelText('Buscar por código o nombre')
    await user.type(search, 'cafe')
    expect(screen.getByText('Café con leche')).toBeInTheDocument()
    expect(screen.queryByText('Pan de queso')).not.toBeInTheDocument()

    await user.clear(search)
    await user.type(search, 'queso')
    expect(screen.getByText('Pan de queso')).toBeInTheDocument()

    await user.clear(search)
    await user.click(await screen.findByRole('button', { name: 'Agregar Café con leche' }))
    expect(screen.getByLabelText('Cantidad de Café con leche')).toHaveTextContent('1')
    expect(document.querySelector('.total-value')).toHaveTextContent('Bs 1.234,56')

    await user.click(screen.getByRole('button', { name: 'Aumentar Café con leche' }))
    expect(screen.getByLabelText('Cantidad de Café con leche')).toHaveTextContent('2')
    expect(document.querySelector('.total-value')).toHaveTextContent('Bs 2.469,12')

    await user.click(screen.getByRole('button', { name: 'Eliminar' }))
    expect(screen.getByText('Agregue productos desde el catálogo.')).toBeInTheDocument()
  })

  it('avisa cuando no hay caja abierta', async () => {
    mockApi(
      withRoutes([
        {
          method: 'GET',
          path: '/api/caja/abierta',
          handler: () => jsonResponse({ caja: null }),
        },
      ]),
    )
    renderVenta()

    expect(await screen.findByText(/No hay caja abierta/)).toBeInTheDocument()
  })

  it('permite completar con un solo método o agregar método secundario', async () => {
    const user = userEvent.setup()
    let partId = 0
    const montos = new Map<number, number>()
    mockApi(
      withRoutes([
        {
          method: 'POST',
          path: '/api/ventas',
          handler: () => jsonResponse(saleFixture(), 201),
        },
        {
          method: 'POST',
          path: '/api/ventas/77/partes',
          handler: (init) => {
            const body = JSON.parse(String(init.body)) as { monto_bs: number; metodo: string }
            partId += 1
            montos.set(partId, body.monto_bs)
            return jsonResponse(
              partFixture({ id: partId, metodo: body.metodo, monto_bs: body.monto_bs }),
              201,
            )
          },
        },
        {
          method: 'POST',
          path: /^\/api\/ventas\/77\/partes\/\d+\/confirmar$/,
          handler: (_init, path) => {
            const id = Number(path.split('/')[5])
            return jsonResponse(
              partFixture({
                id,
                metodo: montos.get(id) === 123456 ? 'tarjeta' : 'efectivo',
                monto_bs: montos.get(id) ?? 0,
                estado: 'confirmado',
              }),
            )
          },
        },
      ]),
    )
    renderVenta()
    await addCafeAndCheckout(user)

    // Pago único
    await user.selectOptions(screen.getByLabelText('Método de pago'), 'tarjeta')
    await user.click(screen.getByRole('button', { name: 'Agregar pago' }))

    const firstPart = (await screen.findAllByText('Tarjeta'))
      .map((element) => element.closest('li'))
      .find((element): element is HTMLLIElement => element !== null) as HTMLElement
    await user.click(within(firstPart).getByRole('button', { name: 'Confirmar' }))

    expect(await screen.findByText('La suma de pagos coincide con el total.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Completar venta' })).toBeEnabled()
  })

  it('muestra el cambio del backend para pagos en efectivo', async () => {
    const user = userEvent.setup()
    mockApi(
      withRoutes([
        {
          method: 'POST',
          path: '/api/ventas',
          handler: () => jsonResponse(saleFixture(), 201),
        },
        {
          method: 'POST',
          path: '/api/ventas/77/partes',
          handler: (init) => {
            const body = JSON.parse(String(init.body)) as {
              monto_bs: number
              recibido_bs: number
            }
            return jsonResponse(
              partFixture({
                metodo: 'efectivo',
                monto_bs: body.monto_bs,
                cambio_bs: (body.recibido_bs ?? body.monto_bs) - body.monto_bs,
              }),
              201,
            )
          },
        },
      ]),
    )
    renderVenta()
    await addCafeAndCheckout(user)

    await user.type(screen.getByLabelText('Efectivo recibido (Bs)'), '2000')
    await user.click(screen.getByRole('button', { name: 'Agregar pago' }))

    expect(await screen.findByText('Cambio: Bs 765,44')).toBeInTheDocument()
  })

  it('rechaza en cliente un efectivo recibido menor que la parte', async () => {
    const user = userEvent.setup()
    const calls = mockApi(
      withRoutes([
        {
          method: 'POST',
          path: '/api/ventas',
          handler: () => jsonResponse(saleFixture(), 201),
        },
      ]),
    )
    renderVenta()
    await addCafeAndCheckout(user)

    await user.type(screen.getByLabelText('Efectivo recibido (Bs)'), '100')
    await user.click(screen.getByRole('button', { name: 'Agregar pago' }))

    expect(await screen.findByText(/efectivo recibido es menor/i)).toBeInTheDocument()
    expect(calls.some((call) => call.path === '/api/ventas/77/partes')).toBe(false)
  })

  it('muestra el mensaje de autorización del backend sin perder el carrito', async () => {
    const user = userEvent.setup()
    mockApi(
      withRoutes([
        {
          method: 'POST',
          path: '/api/ventas',
          handler: () => jsonResponse(saleFixture(), 201),
        },
        {
          method: 'POST',
          path: '/api/ventas/77/partes',
          handler: () =>
            jsonResponse(
              {
                error: {
                  code: 'no_autorizado',
                  message: 'No autorizado para realizar esta operación',
                },
              },
              403,
            ),
        },
      ]),
    )
    renderVenta()
    await addCafeAndCheckout(user)

    await user.type(screen.getByLabelText('Efectivo recibido (Bs)'), '2000')
    await user.click(screen.getByRole('button', { name: 'Agregar pago' }))

    expect(
      await screen.findByText('No autorizado para realizar esta operación'),
    ).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Cobro de la venta #77/ })).toBeInTheDocument()
  })

  it('confirma stock negativo y reintenta con la misma Idempotency-Key', async () => {
    const user = userEvent.setup()
    let completarCalls = 0
    const calls = mockApi(
      withRoutes([
        {
          method: 'POST',
          path: '/api/ventas',
          handler: () => jsonResponse(saleFixture(), 201),
        },
        {
          method: 'POST',
          path: '/api/ventas/77/partes',
          handler: () =>
            jsonResponse(
              partFixture({ id: 1, metodo: 'tarjeta', monto_bs: 123456 }),
              201,
            ),
        },
        {
          method: 'POST',
          path: '/api/ventas/77/partes/1/confirmar',
          handler: () =>
            jsonResponse(
              partFixture({ id: 1, metodo: 'tarjeta', monto_bs: 123456, estado: 'confirmado' }),
            ),
        },
        {
          method: 'POST',
          path: '/api/ventas/77/completar',
          handler: () => {
            completarCalls += 1
            if (completarCalls === 1) {
              return jsonResponse(
                {
                  error: {
                    code: 'stock_insuficiente',
                    message:
                      'La salida dejará el stock en negativo (quedará en -1). ¿Desea continuar?',
                  },
                },
                409,
              )
            }
            return jsonResponse(completionFixture({ stock_negativo: true }))
          },
        },
        {
          method: 'GET',
          path: '/api/ventas/77/comandas',
          handler: () => jsonResponse({ comandas: [] }),
        },
      ]),
    )

    renderVenta()
    await addCafeAndCheckout(user)

    await user.selectOptions(screen.getByLabelText('Método de pago'), 'tarjeta')
    await user.click(screen.getByRole('button', { name: 'Agregar pago' }))
    await user.click(await screen.findByRole('button', { name: 'Confirmar' }))
    await screen.findByText('La suma de pagos coincide con el total.')

    await user.click(screen.getByRole('button', { name: 'Completar venta' }))

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent(/stock en negativo/)
    expect(screen.queryByText('Venta completada')).not.toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Continuar y completar' }))

    expect(await screen.findByText('Venta completada')).toBeInTheDocument()
    const completar = calls.filter((call) => call.path === '/api/ventas/77/completar')
    expect(completar).toHaveLength(2)
    const headers = completar.map(
      (call) => (call.init.headers as Record<string, string>)['Idempotency-Key'],
    )
    expect(headers[0]).toBeTruthy()
    expect(headers[1]).toBe(headers[0])
    const firstBody = JSON.parse(String(completar[0].init.body)) as {
      confirmar_stock_negativo: boolean
    }
    const secondBody = JSON.parse(String(completar[1].init.body)) as {
      confirmar_stock_negativo: boolean
    }
    expect(firstBody.confirmar_stock_negativo).toBe(false)
    expect(secondBody.confirmar_stock_negativo).toBe(true)
  })
})
