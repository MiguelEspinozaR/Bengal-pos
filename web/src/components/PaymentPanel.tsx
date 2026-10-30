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
  const [recibido, setRecibido] = useState('')
  const [metodoSec, setMetodoSec] = useState<MetodoPago>('qr')
  const [montoSec, setMontoSec] = useState('')
  const [recibidoSec, setRecibidoSec] = useState('')
  const [showSecondary, setShowSecondary] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)

  const confirmedTotal = parts
    .filter((part) => part.estado === 'confirmado')
    .reduce((sum, part) => sum + part.monto_bs, 0)
  const allConfirmed = parts.length > 0 && parts.every((part) => part.estado === 'confirmado')
  const sumMatches = confirmedTotal === sale.total_bs
  const canComplete = allConfirmed && sumMatches && busy === null
  const missing = sale.total_bs - confirmedTotal

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setLocalError(null)

    let recibidoBs: number | undefined
    if (metodo === 'efectivo') {
      try {
        recibidoBs = parseBs(recibido)
      } catch {
        setLocalError('Ingrese el efectivo recibido en Bs.')
        return
      }
      if (recibidoBs < sale.total_bs) {
        setLocalError('El efectivo recibido es menor que el total a pagar.')
        return
      }
    }

    await onAddPart({
      metodo,
      monto_bs: sale.total_bs,
      recibido_bs: recibidoBs,
    })
    setRecibido('')
  }

  const handleAddSecondary = async (event: FormEvent) => {
    event.preventDefault()
    setLocalError(null)
    if (missing <= 0) {
      setLocalError('Ya no falta por pagar.')
      return
    }
    let montoBs: number
    try {
      montoBs = parseBs(montoSec)
    } catch {
      setLocalError('Ingrese un monto válido en Bs.')
      return
    }
    if (montoBs <= 0 || montoBs > missing) {
      setLocalError('El monto debe ser mayor que cero y no exceder lo que falta por pagar.')
      return
    }

    let recibidoBsSec: number | undefined
    if (metodoSec === 'efectivo') {
      try {
        recibidoBsSec = parseBs(recibidoSec)
      } catch {
        setLocalError('Ingrese el efectivo recibido para el segundo método.')
        return
      }
      if (recibidoBsSec < montoBs) {
        setLocalError('El efectivo recibido es menor que el monto a cubrir.')
        return
      }
    }

    await onAddPart({
      metodo: metodoSec,
      monto_bs: montoBs,
      recibido_bs: recibidoBsSec,
    })
    setMontoSec('')
    setRecibidoSec('')
  }

  const startSecondary = () => {
    setShowSecondary(true)
    setMontoSec(formatBs(missing).replace('Bs ', '').replace(',', '.'))
  }

  const cambio = metodo === 'efectivo' && recibido ? Math.max(0, parseBsSafe(recibido) - sale.total_bs) : 0

  return (
    <section className="payment-panel" aria-label="Cobro y pagos">
      <div className="panel-heading">
        <h2>Cobro de la venta #{sale.id}</h2>
        <span className="badge">Total a pagar {formatBs(sale.total_bs)}</span>
      </div>

      {parts.length === 0 ? (
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

          {metodo === 'efectivo' ? (
            <>
              <div className="field">
                <label htmlFor="efectivo-recibido">Efectivo recibido (Bs)</label>
                <input
                  id="efectivo-recibido"
                  inputMode="decimal"
                  autoComplete="off"
                  value={recibido}
                  onChange={(event) => setRecibido(event.target.value)}
                  placeholder="0,00"
                />
              </div>
              {recibido ? (
                <div className="total-row">
                  <span>Cambio</span>
                  <strong>{formatBs(cambio)}</strong>
                </div>
              ) : null}
            </>
          ) : null}

          {localError ? (
            <p className="form-error" role="alert">
              {localError}
            </p>
          ) : null}

          <button type="submit" className="btn btn-primary btn-block" disabled={busy !== null}>
            {busy === 'agregar' ? 'Agregando…' : 'Agregar pago'}
          </button>
        </form>
      ) : null}

      {parts.length > 0 ? (
        <div className="parts">
          <h3>Pagos</h3>
          <ul className="parts-list">
            {parts.map((part) => (
              <li key={part.id} className="part-line">
                <div className="part-info">
                  <strong>{METHOD_LABEL[part.metodo] ?? part.metodo}</strong>
                  <span>Cubierto: {formatBs(part.monto_bs)}</span>
                  {part.metodo === 'efectivo' && part.recibido_bs ? (
                    <span className="muted">Recibido: {formatBs(part.recibido_bs)}</span>
                  ) : null}
                  <span className={`status status-${part.estado}`}>{part.estado}</span>
                  {part.cambio_bs > 0 ? (
                    <span className="change">Cambio: {formatBs(part.cambio_bs)}</span>
                  ) : null}
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

          {!showSecondary && parts.length === 1 && parts[0].estado === 'confirmado' && missing > 0 ? (
            <button type="button" className="btn btn-secondary btn-block" onClick={startSecondary}>
              + Agregar método secundario
            </button>
          ) : null}

          {showSecondary && missing > 0 ? (
            <form className="payment-form secondary" onSubmit={(event) => void handleAddSecondary(event)}>
              <h4>Método secundario</h4>
              <p className="muted">Falta por pagar: {formatBs(missing)}</p>
              <div className="field">
                <label htmlFor="metodo-sec">Método</label>
                <select
                  id="metodo-sec"
                  value={metodoSec}
                  onChange={(event) => setMetodoSec(event.target.value as MetodoPago)}
                >
                  <option value="qr">QR</option>
                  <option value="tarjeta">Tarjeta</option>
                  <option value="efectivo">Efectivo</option>
                </select>
              </div>
              <div className="field">
                <label htmlFor="monto-sec">Monto a cubrir (Bs)</label>
                <input
                  id="monto-sec"
                  inputMode="decimal"
                  autoComplete="off"
                  value={montoSec}
                  onChange={(event) => setMontoSec(event.target.value)}
                  placeholder="0,00"
                />
              </div>
              {metodoSec === 'efectivo' ? (
                <div className="field">
                  <label htmlFor="recibido-sec">Efectivo recibido (Bs)</label>
                  <input
                    id="recibido-sec"
                    inputMode="decimal"
                    autoComplete="off"
                    value={recibidoSec}
                    onChange={(event) => setRecibidoSec(event.target.value)}
                    placeholder="0,00"
                  />
                </div>
              ) : null}
              {localError ? (
                <p className="form-error" role="alert">
                  {localError}
                </p>
              ) : null}
              <button type="submit" className="btn btn-secondary btn-block" disabled={busy !== null}>
                {busy === 'agregar' ? 'Agregando…' : 'Agregar método secundario'}
              </button>
            </form>
          ) : null}
        </div>
      ) : null}

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
        ) : confirmedTotal > 0 ? (
          <p className="ok-text">La suma de pagos coincide con el total.</p>
        ) : null}
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

function parseBsSafe(value: string): number {
  try {
    return parseBs(value)
  } catch {
    return 0
  }
}
