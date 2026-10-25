import { useState, type FormEvent } from 'react'
import { formatBs, parseBs } from '../lib/money'
import type { PaymentPart, Sale } from '../types'

export type MetodoPago = 'efectivo' | 'tarjeta' | 'qr'

export interface NuevoPago {
  metodo: MetodoPago
  monto_bs: number
  recibido_bs?: number
  referencia?: string
}

interface PaymentPanelProps {
  sale: Sale
  parts: PaymentPart[]
  busy: string | null
  onAddPart: (input: NuevoPago) => Promise<void>
  onConfirmPart: (parteId: number) => Promise<void>
  onComplete: () => Promise<void>
}

const METHOD_LABEL: Record<string, string> = {
  efectivo: 'Efectivo',
  tarjeta: 'Tarjeta',
  qr: 'QR',
}

export function PaymentPanel({
  sale,
  parts,
  busy,
  onAddPart,
  onConfirmPart,
  onComplete,
}: PaymentPanelProps) {
  const [metodo, setMetodo] = useState<MetodoPago>('efectivo')
  const [monto, setMonto] = useState('')
  const [recibido, setRecibido] = useState('')
  const [referencia, setReferencia] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)

  const confirmedTotal = parts
    .filter((part) => part.estado === 'confirmado')
    .reduce((sum, part) => sum + part.monto_bs, 0)
  const allConfirmed = parts.length > 0 && parts.every((part) => part.estado === 'confirmado')
  const sumMatches = confirmedTotal === sale.total_bs
  const canComplete = allConfirmed && sumMatches && busy === null

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setLocalError(null)
    let montoBs: number
    try {
      montoBs = parseBs(monto)
    } catch {
      setLocalError('Ingrese un monto válido en Bs (máximo dos decimales).')
      return
    }
    if (montoBs <= 0) {
      setLocalError('El monto debe ser mayor que cero.')
      return
    }

    let recibidoBs: number | undefined
    if (metodo === 'efectivo') {
      try {
        recibidoBs = parseBs(recibido)
      } catch {
        setLocalError('Ingrese el efectivo recibido en Bs.')
        return
      }
      if (recibidoBs < montoBs) {
        setLocalError('El efectivo recibido es menor que el monto en efectivo a pagar.')
        return
      }
    }

    await onAddPart({
      metodo,
      monto_bs: montoBs,
      recibido_bs: recibidoBs,
      referencia: referencia.trim() === '' ? undefined : referencia.trim(),
    })
    setMonto('')
    setRecibido('')
    setReferencia('')
  }

  const missing = sale.total_bs - confirmedTotal

  return (
    <section className="payment-panel" aria-label="Cobro y pagos">
      <div className="panel-heading">
        <h2>Cobro de la venta #{sale.id}</h2>
        <span className="badge">Total {formatBs(sale.total_bs)}</span>
      </div>

      <form className="payment-form" onSubmit={(event) => void handleSubmit(event)}>
        <div className="field">
          <label htmlFor="metodo-pago">Método de pago</label>
          <select
            id="metodo-pago"
            value={metodo}
            onChange={(event) => setMetodo(event.target.value as MetodoPago)}
          >
            <option value="efectivo">Efectivo</option>
            <option value="tarjeta">Tarjeta</option>
            <option value="qr">QR</option>
          </select>
        </div>

        <div className="field">
          <label htmlFor="monto-parte">Monto de la parte (Bs)</label>
          <input
            id="monto-parte"
            inputMode="decimal"
            autoComplete="off"
            value={monto}
            onChange={(event) => setMonto(event.target.value)}
            placeholder="0,00"
          />
        </div>

        {metodo === 'efectivo' ? (
          <div className="field">
            <label htmlFor="recibido-efectivo">Efectivo recibido (Bs)</label>
            <input
              id="recibido-efectivo"
              inputMode="decimal"
              autoComplete="off"
              value={recibido}
              onChange={(event) => setRecibido(event.target.value)}
              placeholder="0,00"
            />
          </div>
        ) : null}

        <div className="field">
          <label htmlFor="referencia-pago">Referencia (opcional)</label>
          <input
            id="referencia-pago"
            autoComplete="off"
            value={referencia}
            onChange={(event) => setReferencia(event.target.value)}
          />
        </div>

        {localError ? (
          <p className="form-error" role="alert">
            {localError}
          </p>
        ) : null}

        <button type="submit" className="btn btn-secondary btn-block" disabled={busy !== null}>
          {busy === 'agregar' ? 'Agregando…' : 'Agregar pago'}
        </button>
      </form>

      <div className="parts">
        <h3>Partes de pago</h3>
        {parts.length === 0 ? (
          <p className="muted">Agregue una o varias partes y confírmelas manualmente.</p>
        ) : (
          <ul className="parts-list">
            {parts.map((part) => (
              <li key={part.id} className="part-line">
                <div className="part-info">
                  <strong>{METHOD_LABEL[part.metodo] ?? part.metodo}</strong>
                  <span>{formatBs(part.monto_bs)}</span>
                  <span className={`status status-${part.estado}`}>{part.estado}</span>
                  {part.cambio_bs > 0 ? (
                    <span className="change">Cambio: {formatBs(part.cambio_bs)}</span>
                  ) : null}
                  {part.referencia ? <span className="muted">Ref. {part.referencia}</span> : null}
                </div>
                {part.estado !== 'confirmado' ? (
                  <button
                    type="button"
                    className="btn btn-secondary"
                    onClick={() => void onConfirmPart(part.id)}
                    disabled={busy !== null}
                  >
                    {busy === `confirmar-${part.id}` ? 'Confirmando…' : 'Confirmar'}
                  </button>
                ) : (
                  <span className="confirmed-mark" aria-label={`Parte ${part.id} confirmada`}>
                    ✓ Confirmada
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="payment-summary">
        <div className="total-row">
          <span>Confirmado</span>
          <strong>{formatBs(confirmedTotal)}</strong>
        </div>
        {missing > 0 ? (
          <p className="muted" role="status">
            Falta por confirmar: {formatBs(missing)}
          </p>
        ) : missing < 0 ? (
          <p className="form-error" role="alert">
            Los pagos exceden el total por {formatBs(-missing)}.
          </p>
        ) : (
          <p className="ok-text">La suma de pagos coincide con el total.</p>
        )}
        <button
          type="button"
          className="btn btn-primary btn-block"
          onClick={() => void onComplete()}
          disabled={!canComplete}
        >
          {busy === 'completar' ? 'Completando…' : 'Completar venta'}
        </button>
        {!allConfirmed || !sumMatches ? (
          <p className="muted small">
            Debe confirmar todas las partes y que la suma sea igual al total.
          </p>
        ) : null}
      </div>
    </section>
  )
}
