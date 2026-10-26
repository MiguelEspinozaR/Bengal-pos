import { useCallback, useEffect, useMemo, useState } from 'react'
import { apiGet, apiPost, errorMessage } from '../api/client'
import { AppShell } from '../components/AppShell'
import { formatBs, parseBs } from '../lib/money'
import type { CajaCierreResult, CajaMovimiento, CajaSesion, UsuarioDetalle } from '../types'

interface SesionDetalle extends CajaSesion {
  movimientos?: CajaMovimiento[]
}

function parseMonto(value: string): number | null {
  try {
    const parsed = parseBs(value)
    if (parsed <= 0) return null
    return parsed
  } catch {
    return null
  }
}

export function CajaPage() {
  const [abierta, setAbierta] = useState<CajaSesion | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [aperturaMonto, setAperturaMonto] = useState('0')
  const [aperturaBusy, setAperturaBusy] = useState(false)
  const [aperturaError, setAperturaError] = useState<string | null>(null)

  const [movTipo, setMovTipo] = useState<'entrada' | 'salida'>('entrada')
  const [movMonto, setMovMonto] = useState('')
  const [movMotivo, setMovMotivo] = useState('')
  const [movBusy, setMovBusy] = useState(false)
  const [movError, setMovError] = useState<string | null>(null)

  const [cierreMonto, setCierreMonto] = useState('')
  const [cierreBusy, setCierreBusy] = useState(false)
  const [cierreError, setCierreError] = useState<string | null>(null)
  const [cierreResult, setCierreResult] = useState<CajaCierreResult | null>(null)

  const [historial, setHistorial] = useState<CajaSesion[]>([])
  const [historialLoading, setHistorialLoading] = useState(false)
  const [sesionSeleccionada, setSesionSeleccionada] = useState<SesionDetalle | null>(null)
  const [movimientosLoading, setMovimientosLoading] = useState(false)
  const [usuariosMap, setUsuariosMap] = useState<Record<number, string>>({})

  const loadUsuarios = useCallback(async () => {
    try {
      const data = await apiGet<{ usuarios: UsuarioDetalle[] }>('/api/usuarios?incluir_inactivos=true')
      const map: Record<number, string> = {}
      for (const u of data.usuarios ?? []) {
        map[u.id] = u.nombre_completo
      }
      setUsuariosMap(map)
    } catch {
      // ignore
    }
  }, [])

  const loadAbierta = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await apiGet<{ caja: CajaSesion | null }>('/api/caja/abierta')
      setAbierta(data.caja)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  const loadHistorial = useCallback(async () => {
    setHistorialLoading(true)
    try {
      const data = await apiGet<{ sesiones: CajaSesion[] }>('/api/caja/historial')
      setHistorial(data.sesiones ?? [])
    } catch {
      setHistorial([])
    } finally {
      setHistorialLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadAbierta()
    void loadHistorial()
    void loadUsuarios()
  }, [loadAbierta, loadHistorial, loadUsuarios])

  const usuarioNombre = useCallback(
    (id: number) => usuariosMap[id] ?? `Usuario #${id}`,
    [usuariosMap],
  )

  const handleApertura = async (e: React.FormEvent) => {
    e.preventDefault()
    setAperturaError(null)
    const monto = parseMonto(aperturaMonto)
    if (monto === null) {
      setAperturaError('Monto inválido o negativo.')
      return
    }
    setAperturaBusy(true)
    try {
      const caja = await apiPost<CajaSesion>('/api/caja/apertura', { monto_inicial_bs: monto })
      setAbierta(caja)
      setCierreResult(null)
      await loadHistorial()
    } catch (err) {
      setAperturaError(errorMessage(err))
    } finally {
      setAperturaBusy(false)
    }
  }

  const handleMovimiento = async (e: React.FormEvent) => {
    e.preventDefault()
    setMovError(null)
    if (!abierta) {
      setMovError('Sin caja abierta.')
      return
    }
    if (!movMotivo.trim()) {
      setMovError('El motivo es obligatorio.')
      return
    }
    const monto = parseMonto(movMonto)
    if (monto === null) {
      setMovError('Monto inválido. Debe ser un entero positivo.')
      return
    }
    setMovBusy(true)
    try {
      if (movTipo === 'entrada') {
        await apiPost('/api/caja/entradas', { monto_bs: monto, motivo: movMotivo.trim() })
      } else {
        await apiPost('/api/caja/salidas', { monto_bs: monto, motivo: movMotivo.trim() })
      }
      setMovMonto('')
      setMovMotivo('')
    } catch (err) {
      setMovError(errorMessage(err))
    } finally {
      setMovBusy(false)
    }
  }

  const handleCierre = async (e: React.FormEvent) => {
    e.preventDefault()
    setCierreError(null)
    const monto = parseMonto(cierreMonto)
    if (monto === null) {
      setCierreError('Monto inválido.')
      return
    }
    setCierreBusy(true)
    try {
      const result = await apiPost<CajaCierreResult>('/api/caja/cierre', { efectivo_contado_bs: monto })
      setCierreResult(result)
      setAbierta(null)
      setCierreMonto('')
      await loadHistorial()
    } catch (err) {
      setCierreError(errorMessage(err))
    } finally {
      setCierreBusy(false)
    }
  }

  const verMovimientos = async (sesion: CajaSesion) => {
    setMovimientosLoading(true)
    setSesionSeleccionada({ ...sesion, movimientos: [] })
    try {
      const data = await apiGet<{ movimientos: CajaMovimiento[] }>(`/api/caja/sesiones/${sesion.id}/movimientos`)
      setSesionSeleccionada({ ...sesion, movimientos: data.movimientos ?? [] })
    } catch {
      setSesionSeleccionada({ ...sesion, movimientos: [] })
    } finally {
      setMovimientosLoading(false)
    }
  }

  const diferencia = useMemo(() => {
    if (!cierreResult) return null
    const esperado = cierreResult.efectivo_esperado_bs
    const contado = cierreResult.efectivo_contado_bs
    const diff = cierreResult.diferencia_bs
    if (typeof diff === 'number') return diff
    if (typeof esperado === 'number' && typeof contado === 'number') return contado - esperado
    return null
  }, [cierreResult])

  return (
    <AppShell>
      <section className="page">
        <header className="page-header">
          <div>
            <h1>Caja</h1>
            <p className="muted">Sesión, movimientos, cierre e historial.</p>
          </div>
        </header>

        {error ? (
          <div className="card">
            <p className="form-error" role="alert">
              {error}
            </p>
          </div>
        ) : null}

        <div className="grid-two">
          <article className="card">
            <header className="section-header">
              <h2>Estado de caja</h2>
              {loading ? <span className="muted">Cargando…</span> : null}
            </header>
            {!loading && !abierta ? (
              <form onSubmit={(e) => void handleApertura(e)} className="stack">
                <p>No hay caja abierta.</p>
                <div className="field">
                  <label htmlFor="monto-inicial">Monto inicial (Bs)</label>
                  <input
                    id="monto-inicial"
                    inputMode="decimal"
                    value={aperturaMonto}
                    onChange={(e) => setAperturaMonto(e.target.value)}
                    placeholder="0,00"
                  />
                </div>
                {aperturaError ? (
                  <p className="form-error" role="alert">
                    {aperturaError}
                  </p>
                ) : null}
                <div className="form-actions">
                  <button className="btn btn-primary" type="submit" disabled={aperturaBusy}>
                    {aperturaBusy ? 'Abriendo…' : 'Abrir caja'}
                  </button>
                </div>
              </form>
            ) : null}
            {!loading && abierta ? (
              <div className="stack">
                <div className="grid-two">
                  <div>
                    <p className="muted">Sesión #{abierta.id}</p>
                    <p>Estado: {abierta.estado}</p>
                    <p>Monto inicial: {formatBs(abierta.monto_inicial_bs)}</p>
                  <p>Fecha apertura: {abierta.fecha_apertura ? new Date(abierta.fecha_apertura).toLocaleString() : '—'}</p>
                  <p>Abierta por: {typeof abierta.usuario_apertura_id === 'number' ? usuarioNombre(abierta.usuario_apertura_id) : '—'}</p>
                  </div>
                </div>
              </div>
            ) : null}
          </article>

          <article className="card">
            <header className="section-header">
              <h2>Entradas / salidas</h2>
            </header>
            {!abierta ? (
              <p className="muted">Sin caja abierta. No se habilitan los formularios.</p>
            ) : (
              <form onSubmit={(e) => void handleMovimiento(e)} className="stack">
                <div className="form-row">
                  <div className="field">
                    <label htmlFor="mov-tipo">Tipo</label>
                    <select id="mov-tipo" value={movTipo} onChange={(e) => setMovTipo(e.target.value as 'entrada' | 'salida')}>
                      <option value="entrada">Entrada</option>
                      <option value="salida">Salida</option>
                    </select>
                  </div>
                  <div className="field">
                    <label htmlFor="mov-monto">Monto (Bs)</label>
                    <input
                      id="mov-monto"
                      inputMode="decimal"
                      value={movMonto}
                      onChange={(e) => setMovMonto(e.target.value)}
                      placeholder="1.000,00"
                    />
                  </div>
                </div>
                <div className="field">
                  <label htmlFor="mov-motivo">Motivo</label>
                  <input id="mov-motivo" value={movMotivo} onChange={(e) => setMovMotivo(e.target.value)} required />
                </div>
                {movError ? (
                  <p className="form-error" role="alert">
                    {movError}
                  </p>
                ) : null}
                <div className="form-actions">
                  <button className="btn btn-primary" type="submit" disabled={movBusy}>
                    {movBusy ? 'Registrando…' : 'Registrar movimiento'}
                  </button>
                </div>
              </form>
            )}
          </article>
        </div>

        <article className="card">
          <header className="section-header">
            <h2>Cierre de caja</h2>
          </header>
          {!abierta ? (
            <p className="muted">No hay caja abierta para cerrar.</p>
          ) : (
            <form onSubmit={(e) => void handleCierre(e)} className="stack">
              <div className="field">
                <label htmlFor="efectivo-contado">Efectivo contado (Bs)</label>
                <input
                  id="efectivo-contado"
                  inputMode="decimal"
                  value={cierreMonto}
                  onChange={(e) => setCierreMonto(e.target.value)}
                  placeholder="0,00"
                />
              </div>
              {cierreError ? (
                <p className="form-error" role="alert">
                  {cierreError}
                </p>
              ) : null}
              <div className="form-actions">
                <button className="btn btn-primary" type="submit" disabled={cierreBusy}>
                  {cierreBusy ? 'Cerrando…' : 'Cerrar caja'}
                </button>
              </div>
            </form>
          )}
          {cierreResult ? (
            <div className="stack">
              <h3>Resultado del cierre</h3>
              <p>Esperado: {typeof cierreResult.efectivo_esperado_bs === 'number' ? formatBs(cierreResult.efectivo_esperado_bs) : '—'}</p>
              <p>Contado: {typeof cierreResult.efectivo_contado_bs === 'number' ? formatBs(cierreResult.efectivo_contado_bs) : '—'}</p>
              <p>Diferencia: {typeof diferencia === 'number' ? formatBs(diferencia) : '—'}</p>
              {cierreResult.ventas_en_proceso > 0 ? (
                <div>
                  <p className="form-error" role="alert">
                    {cierreResult.aviso || `Hay ${cierreResult.ventas_en_proceso} venta(s) en proceso.`}
                  </p>
                  {(cierreResult.ventas_en_proceso_detalle ?? []).map((v) => (
                    <p key={v.id} className="muted small">
                      Venta #{v.id} - {usuarioNombre(v.usuario_id)} - {formatBs(v.total_bs)} - {new Date(v.fecha).toLocaleString()}
                    </p>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
        </article>

        <article className="card">
          <header className="section-header">
            <h2>Historial de sesiones</h2>
            {historialLoading ? <span className="muted">Cargando…</span> : null}
          </header>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Sesión</th>
                  <th>Estado</th>
                  <th>Apertura</th>
                  <th>Cierre</th>
                  <th>Inicial</th>
                  <th>Esperado</th>
                  <th>Contado</th>
                  <th>Diferencia</th>
                  <th>Acciones</th>
                </tr>
              </thead>
              <tbody>
                {(historial ?? []).map((s) => (
                  <tr key={s.id}>
                    <td>#{s.id}</td>
                    <td>{s.estado}</td>
                    <td>{s.fecha_apertura ? new Date(s.fecha_apertura).toLocaleString() : '—'}</td>
                    <td>{s.fecha_cierre ? new Date(s.fecha_cierre).toLocaleString() : '—'}</td>
                    <td>{formatBs(s.monto_inicial_bs)}</td>
                    <td>{typeof s.efectivo_esperado_bs === 'number' ? formatBs(s.efectivo_esperado_bs) : '—'}</td>
                    <td>{typeof s.efectivo_contado_bs === 'number' ? formatBs(s.efectivo_contado_bs) : '—'}</td>
                    <td>{typeof s.diferencia_bs === 'number' ? formatBs(s.diferencia_bs) : '—'}</td>
                    <td>
                      <button className="btn btn-small" type="button" onClick={() => void verMovimientos(s)}>
                        Ver movimientos
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {(historial ?? []).length === 0 && !historialLoading ? <p className="muted">Sin historial.</p> : null}
        </article>

        {sesionSeleccionada ? (
          <article className="card">
            <header className="section-header">
              <h2>Movimientos de sesión #{sesionSeleccionada.id}</h2>
              <button className="btn btn-small" type="button" onClick={() => setSesionSeleccionada(null)}>
                Cerrar detalle
              </button>
            </header>
            {movimientosLoading ? <p className="muted">Cargando…</p> : null}
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Tipo</th>
                    <th>Monto</th>
                    <th>Motivo</th>
                    <th>Usuario</th>
                    <th>Venta</th>
                    <th>Fecha</th>
                  </tr>
                </thead>
                <tbody>
                  {(sesionSeleccionada.movimientos ?? []).map((m) => (
                    <tr key={m.id}>
                      <td>{m.tipo}</td>
                      <td>{formatBs(m.monto_bs)}</td>
                      <td>{m.motivo || '—'}</td>
                      <td>{usuarioNombre(m.usuario_id)}</td>
                      <td>{m.venta_id ? `#${m.venta_id}` : '—'}</td>
                      <td>{new Date(m.creado_en).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {(sesionSeleccionada.movimientos ?? []).length === 0 && !movimientosLoading ? <p className="muted">Sin movimientos.</p> : null}
          </article>
        ) : null}
      </section>
    </AppShell>
  )
}
