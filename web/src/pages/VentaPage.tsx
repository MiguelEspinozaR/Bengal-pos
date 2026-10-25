import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError, apiGet, apiPost, errorMessage } from '../api/client'
import { AppShell } from '../components/AppShell'
import { CartPanel } from '../components/CartPanel'
import { PaymentPanel, type NuevoPago } from '../components/PaymentPanel'
import { StockNegativeDialog } from '../components/StockNegativeDialog'
import { useIsMobile } from '../hooks/useMediaQuery'
import { newIdempotencyKey } from '../lib/idempotency'
import { formatBs } from '../lib/money'
import { matchesProduct } from '../lib/text'
import type {
  CajaSesion,
  CartLine,
  Deposito,
  PaymentPart,
  Producto,
  Sale,
  SaleCompletion,
  SaleCompletionView,
} from '../types'

export function VentaPage() {
  const navigate = useNavigate()
  const isMobile = useIsMobile()

  const [productos, setProductos] = useState<Producto[]>([])
  const [productosLoading, setProductosLoading] = useState(true)
  const [productosError, setProductosError] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const [depositos, setDepositos] = useState<Deposito[]>([])
  const [depositoID, setDepositoID] = useState<number | null>(null)

  const [caja, setCaja] = useState<CajaSesion | null>(null)
  const [cajaChecked, setCajaChecked] = useState(false)
  const [cajaError, setCajaError] = useState<string | null>(null)

  const [lines, setLines] = useState<CartLine[]>([])
  const [cartOpen, setCartOpen] = useState(false)

  const [sale, setSale] = useState<Sale | null>(null)
  const [parts, setParts] = useState<PaymentPart[]>([])
  const [idempotencyKey, setIdempotencyKey] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [stockWarning, setStockWarning] = useState<string | null>(null)

  const loadProductos = useCallback(async () => {
    setProductosLoading(true)
    setProductosError(null)
    try {
      const data = await apiGet<{ productos: Producto[] }>('/api/productos')
      setProductos((data.productos ?? []).filter((producto) => producto.activo))
    } catch (err) {
      setProductosError(errorMessage(err))
    } finally {
      setProductosLoading(false)
    }
  }, [])

  const loadCaja = useCallback(async () => {
    setCajaError(null)
    try {
      const data = await apiGet<{ caja: CajaSesion | null }>('/api/caja/abierta')
      setCaja(data.caja)
    } catch (err) {
      setCaja(null)
      setCajaError(errorMessage(err))
    } finally {
      setCajaChecked(true)
    }
  }, [])

  useEffect(() => {
    void loadProductos()
    void loadCaja()
  }, [loadProductos, loadCaja])

  useEffect(() => {
    let active = true
    apiGet<{ depositos: Deposito[] }>('/api/depositos')
      .then((data) => {
        if (active) setDepositos((data.depositos ?? []).filter((deposito) => deposito.activo))
      })
      .catch(() => {
        // The deposit is optional; completion falls back to the backend default.
      })
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    if (depositoID === null && depositos.length > 0) {
      setDepositoID(depositos[0].id)
    }
  }, [depositoID, depositos])

  const filtered = useMemo(
    () => productos.filter((producto) => matchesProduct(producto, search)),
    [productos, search],
  )

  const total = useMemo(
    () =>
      lines.reduce(
        (sum, line) => sum + line.producto.precio_venta_bs * line.cantidad,
        0,
      ),
    [lines],
  )

  const itemCount = lines.reduce((sum, line) => sum + line.cantidad, 0)

  const addProduct = (producto: Producto) => {
    if (sale) return
    setLines((current) => {
      const existing = current.find((line) => line.producto.id === producto.id)
      if (existing) {
        return current.map((line) =>
          line.producto.id === producto.id
            ? { ...line, cantidad: line.cantidad + 1 }
            : line,
        )
      }
      return [...current, { producto, cantidad: 1 }]
    })
  }

  const increment = (productoId: number) => {
    if (sale) return
    setLines((current) =>
      current.map((line) =>
        line.producto.id === productoId
          ? { ...line, cantidad: line.cantidad + 1 }
          : line,
      ),
    )
  }

  const decrement = (productoId: number) => {
    if (sale) return
    setLines((current) =>
      current.flatMap((line) => {
        if (line.producto.id !== productoId) return [line]
        if (line.cantidad <= 1) return []
        return [{ ...line, cantidad: line.cantidad - 1 }]
      }),
    )
  }

  const remove = (productoId: number) => {
    if (sale) return
    setLines((current) => current.filter((line) => line.producto.id !== productoId))
  }

  const handleCobrar = async () => {
    if (sale || lines.length === 0) return
    setBusy('crear')
    setError(null)
    const key = newIdempotencyKey()
    try {
      const created = await apiPost<Sale>('/api/ventas', {
        items: lines.map((line) => ({
          producto_id: line.producto.id,
          cantidad: line.cantidad,
        })),
      })
      setSale(created)
      setParts(created.partes ?? [])
      setIdempotencyKey(key)
      setCartOpen(false)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(null)
    }
  }

  const handleAddPart = async (input: NuevoPago) => {
    if (!sale) return
    setBusy('agregar')
    setError(null)
    try {
      const part = await apiPost<PaymentPart>(`/api/ventas/${sale.id}/partes`, input)
      setParts((current) => [...current, part])
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(null)
    }
  }

  const handleConfirmPart = async (parteId: number) => {
    if (!sale) return
    setBusy(`confirmar-${parteId}`)
    setError(null)
    try {
      const updated = await apiPost<PaymentPart>(
        `/api/ventas/${sale.id}/partes/${parteId}/confirmar`,
        {},
      )
      setParts((current) =>
        current.map((part) =>
          part.id === parteId
            ? { ...part, ...updated, cambio_bs: part.cambio_bs }
            : part,
        ),
      )
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(null)
    }
  }

  const completeSale = async (confirmarStockNegativo: boolean) => {
    if (!sale || !idempotencyKey) return
    setBusy('completar')
    setError(null)
    setStockWarning(null)
    try {
      const completion = await apiPost<SaleCompletion>(
        `/api/ventas/${sale.id}/completar`,
        {
          deposito_id: depositoID,
          confirmar_stock_negativo: confirmarStockNegativo,
        },
        { 'Idempotency-Key': idempotencyKey },
      )
      const cambioTotal = parts.reduce(
        (sum, part) => sum + (part.metodo === 'efectivo' ? part.cambio_bs : 0),
        0,
      )
      const view: SaleCompletionView = { completion, cambioTotal }
      navigate(`/venta/${sale.id}/completada`, { replace: true, state: view })
    } catch (err) {
      if (
        err instanceof ApiError &&
        (err.code === 'stock_insuficiente' || err.message.includes('stock en negativo'))
      ) {
        setStockWarning(err.message)
      } else {
        setError(errorMessage(err))
      }
    } finally {
      setBusy(null)
    }
  }

  const showCart = !isMobile || cartOpen

  return (
    <AppShell
      headerAction={
        isMobile ? (
          <button
            type="button"
            className="btn btn-secondary cart-toggle"
            onClick={() => setCartOpen(true)}
          >
            Carrito ({itemCount})
          </button>
        ) : undefined
      }
    >
      <div className="venta-page">
        {cajaChecked && caja === null ? (
          <div className="notice notice-warning" role="status">
            No hay caja abierta. Para cobrar en efectivo, abra la caja primero.
            <button type="button" className="btn btn-ghost btn-small" onClick={() => void loadCaja()}>
              Verificar de nuevo
            </button>
          </div>
        ) : null}
        {cajaError ? (
          <div className="notice notice-error" role="alert">
            No se pudo verificar el estado de la caja: {cajaError}
          </div>
        ) : null}

        {error ? (
          <div className="notice notice-error" role="alert">
            <span>{error}</span>
            <button
              type="button"
              className="btn btn-ghost btn-small"
              aria-label="Cerrar aviso"
              onClick={() => setError(null)}
            >
              ×
            </button>
          </div>
        ) : null}

        <div className="venta-grid">
          <div className="venta-main">
            {sale ? (
              <>
                {depositos.length > 0 ? (
                  <div className="field deposito-field">
                    <label htmlFor="deposito">Depósito de salida</label>
                    <select
                      id="deposito"
                      value={depositoID ?? ''}
                      onChange={(event) => setDepositoID(Number(event.target.value))}
                    >
                      {depositos.map((deposito) => (
                        <option key={deposito.id} value={deposito.id}>
                          {deposito.nombre}
                        </option>
                      ))}
                    </select>
                  </div>
                ) : null}
                <PaymentPanel
                  sale={sale}
                  parts={parts}
                  busy={busy}
                  onAddPart={handleAddPart}
                  onConfirmPart={handleConfirmPart}
                  onComplete={() => completeSale(false)}
                />
              </>
            ) : (
              <section className="catalog-panel" aria-label="Catálogo de productos">
                <div className="panel-heading">
                  <h2>Productos</h2>
                  <span className="muted">{filtered.length} disponibles</span>
                </div>
                <div className="field">
                  <label htmlFor="buscar-producto">Buscar por código o nombre</label>
                  <input
                    id="buscar-producto"
                    type="search"
                    autoComplete="off"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder="Ej. café o P001"
                  />
                </div>

                {productosLoading ? <p className="muted">Cargando productos…</p> : null}
                {productosError ? (
                  <div className="notice notice-error" role="alert">
                    <span>{productosError}</span>
                    <button
                      type="button"
                      className="btn btn-secondary btn-small"
                      onClick={() => void loadProductos()}
                    >
                      Reintentar
                    </button>
                  </div>
                ) : null}
                {!productosLoading && !productosError && filtered.length === 0 ? (
                  <p className="muted">No hay productos que coincidan con la búsqueda.</p>
                ) : null}

                <ul className="product-list">
                  {filtered.map((producto) => (
                    <li key={producto.id} className="product-card">
                      <div className="product-info">
                        <strong>{producto.nombre}</strong>
                        <span className="muted">{producto.codigo}</span>
                      </div>
                      <div className="product-actions">
                        <span className="product-price">{formatBs(producto.precio_venta_bs)}</span>
                        <button
                          type="button"
                          className="btn btn-primary"
                          onClick={() => addProduct(producto)}
                          disabled={sale !== null}
                          aria-label={`Agregar ${producto.nombre}`}
                        >
                          Agregar
                        </button>
                      </div>
                    </li>
                  ))}
                </ul>
              </section>
            )}
          </div>

          {showCart ? (
            <aside className={isMobile ? 'cart-aside cart-drawer' : 'cart-aside'}>
              <CartPanel
                lines={lines}
                total={total}
                locked={sale !== null}
                busy={busy === 'crear'}
                onIncrement={increment}
                onDecrement={decrement}
                onRemove={remove}
                onCheckout={() => void handleCobrar()}
                onClose={isMobile ? () => setCartOpen(false) : undefined}
              />
            </aside>
          ) : null}
        </div>

        {isMobile && cartOpen ? (
          <button
            type="button"
            className="drawer-backdrop"
            aria-label="Cerrar carrito"
            onClick={() => setCartOpen(false)}
          />
        ) : null}
      </div>

      {stockWarning ? (
        <StockNegativeDialog
          message={stockWarning}
          busy={busy === 'completar'}
          onConfirm={() => void completeSale(true)}
          onCancel={() => setStockWarning(null)}
        />
      ) : null}
    </AppShell>
  )
}
