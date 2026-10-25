import { formatBs } from '../lib/money'
import type { CartLine } from '../types'

interface CartPanelProps {
  lines: CartLine[]
  total: number
  locked: boolean
  busy: boolean
  onIncrement: (productoId: number) => void
  onDecrement: (productoId: number) => void
  onRemove: (productoId: number) => void
  onCheckout: () => void
  onClose?: () => void
}

export function CartPanel({
  lines,
  total,
  locked,
  busy,
  onIncrement,
  onDecrement,
  onRemove,
  onCheckout,
  onClose,
}: CartPanelProps) {
  const itemCount = lines.reduce((sum, line) => sum + line.cantidad, 0)

  return (
    <section className="cart-panel" aria-label="Carrito de venta">
      <div className="panel-heading">
        <h2>Carrito ({itemCount})</h2>
        {onClose ? (
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cerrar carrito
          </button>
        ) : null}
      </div>

      {lines.length === 0 ? (
        <p className="muted">Agregue productos desde el catálogo.</p>
      ) : (
        <ul className="cart-list">
          {lines.map((line) => (
            <li key={line.producto.id} className="cart-line">
              <div className="cart-line-info">
                <strong>{line.producto.nombre}</strong>
                <span className="muted">
                  {line.producto.codigo} · {formatBs(line.producto.precio_venta_bs)} c/u
                </span>
              </div>
              <div className="qty-controls">
                <button
                  type="button"
                  className="icon-btn"
                  aria-label={`Disminuir ${line.producto.nombre}`}
                  onClick={() => onDecrement(line.producto.id)}
                  disabled={locked}
                >
                  −
                </button>
                <span className="qty" aria-label={`Cantidad de ${line.producto.nombre}`}>
                  {line.cantidad}
                </span>
                <button
                  type="button"
                  className="icon-btn"
                  aria-label={`Aumentar ${line.producto.nombre}`}
                  onClick={() => onIncrement(line.producto.id)}
                  disabled={locked}
                >
                  +
                </button>
              </div>
              <div className="cart-line-total">
                <strong>{formatBs(line.producto.precio_venta_bs * line.cantidad)}</strong>
                <button
                  type="button"
                  className="btn btn-danger btn-small"
                  onClick={() => onRemove(line.producto.id)}
                  disabled={locked}
                >
                  Eliminar
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <div className="cart-footer">
        <div className="total-row">
          <span>Total</span>
          <strong className="total-value">{formatBs(total)}</strong>
        </div>
        <button
          type="button"
          className="btn btn-primary btn-block"
          onClick={onCheckout}
          disabled={locked || busy || lines.length === 0}
        >
          {locked ? 'Venta en proceso' : busy ? 'Creando venta…' : 'Cobrar'}
        </button>
        {locked ? (
          <p className="muted small">El carrito está bloqueado porque la venta ya fue creada.</p>
        ) : null}
      </div>
    </section>
  )
}
