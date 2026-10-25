import { useEffect, useState } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { apiGet, apiPrintHtml, errorMessage } from '../api/client'
import { AppShell } from '../components/AppShell'
import { formatBs } from '../lib/money'
import { printDocuments, printHtml } from '../lib/print'
import type { Comanda, Sale, SaleCompletionView } from '../types'

export function VentaCompletadaPage() {
  const params = useParams()
  const location = useLocation()
  const view = location.state as SaleCompletionView | null
  const completion = view?.completion ?? null
  const ventaId = Number(params.id)

  const [sale, setSale] = useState<Sale | null>(completion?.venta ?? null)
  const [comandas, setComandas] = useState<Comanda[]>(completion?.comandas ?? [])
  const [loading, setLoading] = useState(completion === null)
  const [error, setError] = useState<string | null>(null)
  const [printError, setPrintError] = useState<string | null>(null)
  const [printing, setPrinting] = useState<string | null>(null)

  useEffect(() => {
    let active = true
    const load = async () => {
      if (!completion) {
        try {
          const fetched = await apiGet<Sale>(`/api/ventas/${ventaId}`)
          if (active) setSale(fetched)
        } catch (err) {
          if (active) setError(errorMessage(err))
        }
      }
      try {
        const data = await apiGet<{ comandas: Comanda[] }>(
          `/api/ventas/${ventaId}/comandas`,
        )
        if (active) setComandas(data.comandas ?? [])
      } catch (err) {
        if (active) setPrintError(errorMessage(err))
      } finally {
        if (active) setLoading(false)
      }
    }
    void load()
    return () => {
      active = false
    }
  }, [ventaId, completion])

  const handlePrintReceipt = async () => {
    setPrintError(null)
    setPrinting('recibo')
    try {
      await printHtml(() =>
        apiPrintHtml(`/api/ventas/${ventaId}/recibo/imprimir?formato=html`),
      )
    } catch (err) {
      setPrintError(errorMessage(err))
    } finally {
      setPrinting(null)
    }
  }

  const handlePrintComandas = async () => {
    if (comandas.length === 0) return
    setPrintError(null)
    setPrinting('comandas')
    try {
      await printDocuments(
        comandas.map((comanda) => () =>
          apiPrintHtml(`/api/comandas/${comanda.id}/imprimir?formato=html`),
        ),
      )
    } catch (err) {
      setPrintError(errorMessage(err))
    } finally {
      setPrinting(null)
    }
  }

  return (
    <AppShell>
      <div className="success-page">
        <div className="success-card">
          <span className="success-icon" aria-hidden="true">
            ✓
          </span>
          <h1>Venta completada</h1>
          <p className="success-number">Venta #{ventaId}</p>

          {loading ? <p className="muted">Cargando datos de la venta…</p> : null}
          {error ? (
            <div className="notice notice-error" role="alert">
              {error}
            </div>
          ) : null}

          {sale ? (
            <dl className="success-summary">
              <div>
                <dt>Total</dt>
                <dd>{formatBs(sale.total_bs)}</dd>
              </div>
              {view ? (
                <div>
                  <dt>Cambio total</dt>
                  <dd>{formatBs(view.cambioTotal)}</dd>
                </div>
              ) : null}
              <div>
                <dt>Estado</dt>
                <dd>{sale.estado}</dd>
              </div>
            </dl>
          ) : null}

          {completion?.stock_negativo ? (
            <div className="notice notice-warning" role="status">
              La venta se completó con stock negativo confirmado.
            </div>
          ) : null}

          {completion && completion.sin_area.length > 0 ? (
            <div className="notice notice-warning" role="status">
              Sin comanda por área: {completion.sin_area.join(', ')}.
            </div>
          ) : null}

          {printError ? (
            <div className="notice notice-error" role="alert">
              {printError}
            </div>
          ) : null}

          <div className="success-actions">
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => void handlePrintReceipt()}
              disabled={printing !== null}
            >
              {printing === 'recibo' ? 'Preparando recibo…' : 'Imprimir recibo'}
            </button>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => void handlePrintComandas()}
              disabled={printing !== null || comandas.length === 0}
            >
              {printing === 'comandas' ? 'Preparando comandas…' : 'Imprimir comandas'}
            </button>
            <Link className="btn btn-ghost" to="/venta">
              Nueva venta
            </Link>
          </div>

          {comandas.length > 0 ? (
            <p className="muted small">
              {comandas.length} comanda{comandas.length === 1 ? '' : 's'} disponible
              {comandas.length === 1 ? '' : 's'} para imprimir.
            </p>
          ) : null}
          <p className="muted small">
            Si el navegador bloquea la ventana de impresión, permita las ventanas emergentes y
            vuelva a intentar. La venta ya está registrada.
          </p>
        </div>
      </div>
    </AppShell>
  )
}
