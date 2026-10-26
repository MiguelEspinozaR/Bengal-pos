import { useCallback, useEffect, useMemo, useState } from 'react'
import { ApiError, apiGet, apiPost, apiPut, errorMessage } from '../api/client'
import { AppShell } from '../components/AppShell'
import { formatBs, parseBs } from '../lib/money'
import type {
  BajoStockGrupo,
  BajoStockReporte,
  DepositoDetalle,
  InventarioMovimiento,
  MovimientoResultado,
  ProductoDetalle,
  StockProducto,
  TrasladoResultado,
  UsuarioDetalle,
} from '../types'

type Tab = 'productos' | 'depositos' | 'stock' | 'movimientos' | 'traslados' | 'bajostock'

function parseMontoPositivo(value: string): number | null {
  try {
    const parsed = parseBs(value)
    if (parsed <= 0) return null
    return parsed
  } catch {
    return null
  }
}

function parseEnteroNoNegativo(value: string): number | null {
  try {
    const parsed = parseBs(value)
    if (parsed < 0) return null
    return Math.floor(parsed / 1)
  } catch {
    return null
  }
}

export function InventarioPage() {
  const [tab, setTab] = useState<Tab>('productos')

  const [productos, setProductos] = useState<ProductoDetalle[]>([])
  const [productosLoading, setProductosLoading] = useState(false)
  const [incluirInactivosProd, setIncluirInactivosProd] = useState(false)

  const [depositos, setDepositos] = useState<DepositoDetalle[]>([])
  const [depositosLoading, setDepositosLoading] = useState(false)
  const [incluirInactivosDep, setIncluirInactivosDep] = useState(false)

  const [stock, setStock] = useState<StockProducto[]>([])
  const [stockLoading, setStockLoading] = useState(false)
  const [filtroProdStock, setFiltroProdStock] = useState('')
  const [filtroDepStock, setFiltroDepStock] = useState('')

  const [movs, setMovs] = useState<InventarioMovimiento[]>([])
  const [movsLoading, setMovsLoading] = useState(false)
  const [filtroProdMov, setFiltroProdMov] = useState('')
  const [filtroDepMov, setFiltroDepMov] = useState('')

  const [bajoStock, setBajoStock] = useState<BajoStockGrupo[]>([])
  const [bajoStockLoading, setBajoStockLoading] = useState(false)

  const [usuariosMap, setUsuariosMap] = useState<Record<number, string>>({})

  const loadUsuarios = useCallback(async () => {
    try {
      const data = await apiGet<{ usuarios: UsuarioDetalle[] }>('/api/usuarios?incluir_inactivos=true')
      const map: Record<number, string> = {}
      for (const u of data.usuarios ?? []) map[u.id] = u.nombre_completo
      setUsuariosMap(map)
    } catch {
      // ignore
    }
  }, [])

  const loadProductos = useCallback(async () => {
    setProductosLoading(true)
    try {
      const data = await apiGet<{ productos: ProductoDetalle[] }>(`/api/productos?incluir_inactivos=${incluirInactivosProd ? 'true' : 'false'}`)
      setProductos(data.productos ?? [])
    } catch {
      setProductos([])
    } finally {
      setProductosLoading(false)
    }
  }, [incluirInactivosProd])

  const loadDepositos = useCallback(async () => {
    setDepositosLoading(true)
    try {
      const data = await apiGet<{ depositos: DepositoDetalle[] }>(`/api/depositos?incluir_inactivos=${incluirInactivosDep ? 'true' : 'false'}`)
      setDepositos(data.depositos ?? [])
    } catch {
      setDepositos([])
    } finally {
      setDepositosLoading(false)
    }
  }, [incluirInactivosDep])

  const loadStock = useCallback(async () => {
    setStockLoading(true)
    try {
      const params = new URLSearchParams()
      if (filtroProdStock) params.set('producto_id', filtroProdStock)
      if (filtroDepStock) params.set('deposito_id', filtroDepStock)
      const qs = params.toString()
      const data = await apiGet<{ stock: StockProducto[] }>(`/api/inventario/stock${qs ? `?${qs}` : ''}`)
      setStock(data.stock ?? [])
    } catch {
      setStock([])
    } finally {
      setStockLoading(false)
    }
  }, [filtroProdStock, filtroDepStock])

  const loadMovs = useCallback(async () => {
    setMovsLoading(true)
    try {
      const params = new URLSearchParams()
      if (filtroProdMov) params.set('producto_id', filtroProdMov)
      if (filtroDepMov) params.set('deposito_id', filtroDepMov)
      const qs = params.toString()
      const data = await apiGet<{ movimientos: InventarioMovimiento[] }>(`/api/inventario/movimientos${qs ? `?${qs}` : ''}`)
      setMovs((data.movimientos ?? []).slice().reverse())
    } catch {
      setMovs([])
    } finally {
      setMovsLoading(false)
    }
  }, [filtroProdMov, filtroDepMov])

  const loadBajoStock = useCallback(async () => {
    setBajoStockLoading(true)
    try {
      const data = await apiGet<BajoStockReporte>('/api/inventario/bajo-stock')
      setBajoStock(data.grupos ?? [])
    } catch {
      setBajoStock([])
    } finally {
      setBajoStockLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadUsuarios()
  }, [loadUsuarios])

  useEffect(() => {
    if (tab === 'productos') void loadProductos()
    if (tab === 'depositos') void loadDepositos()
    if (tab === 'stock') void loadStock()
    if (tab === 'movimientos') void loadMovs()
    if (tab === 'bajostock') void loadBajoStock()
  }, [tab, loadProductos, loadDepositos, loadStock, loadMovs, loadBajoStock])

  const usuarioNombre = useCallback((id: number) => usuariosMap[id] ?? `Usuario #${id}`, [usuariosMap])

  // Producto form
  const [prodForm, setProdForm] = useState({ codigo: '', nombre: '', unidad: '', stock_minimo: '0', precio_venta_bs: '0' })
  const [prodEditId, setProdEditId] = useState<number | null>(null)
  const [prodBusy, setProdBusy] = useState(false)
  const [prodError, setProdError] = useState<string | null>(null)
  const [prodMsg, setProdMsg] = useState<string | null>(null)

  const abrirNuevoProducto = () => {
    setProdEditId(null)
    setProdForm({ codigo: '', nombre: '', unidad: '', stock_minimo: '0', precio_venta_bs: '0' })
    setProdError(null)
    setProdMsg(null)
  }

  const editarProducto = (p: ProductoDetalle) => {
    setProdEditId(p.id)
    setProdForm({
      codigo: p.codigo,
      nombre: p.nombre,
      unidad: p.unidad || '',
      stock_minimo: String(p.stock_minimo ?? 0),
      precio_venta_bs: String(p.precio_venta_bs ?? 0),
    })
    setProdError(null)
    setProdMsg(null)
  }

  const guardarProducto = async (e: React.FormEvent) => {
    e.preventDefault()
    setProdError(null)
    setProdMsg(null)
    const precio = parseEnteroNoNegativo(prodForm.precio_venta_bs)
    const stockMin = parseEnteroNoNegativo(prodForm.stock_minimo)
    if (precio === null || stockMin === null) {
      setProdError('Precio y stock mínimo deben ser enteros no negativos.')
      return
    }
    if (!prodForm.codigo.trim() || !prodForm.nombre.trim()) {
      setProdError('Código y nombre son obligatorios.')
      return
    }
    setProdBusy(true)
    try {
      if (prodEditId === null) {
        const creado = await apiPost<ProductoDetalle>('/api/productos', {
          codigo: prodForm.codigo.trim(),
          nombre: prodForm.nombre.trim(),
          unidad: prodForm.unidad.trim() || undefined,
          stock_minimo: stockMin,
          precio_venta_bs: precio,
        })
        setProdMsg('Producto creado. El stock inicial se registra con una entrada en Movimientos.')
        await loadProductos()
        setProdEditId(creado.id)
      } else {
        await apiPut(`/api/productos/${prodEditId}`, {
          codigo: prodForm.codigo.trim(),
          nombre: prodForm.nombre.trim(),
          unidad: prodForm.unidad.trim() || undefined,
          stock_minimo: stockMin,
          precio_venta_bs: precio,
        })
        setProdMsg('Producto actualizado.')
        await loadProductos()
      }
    } catch (err) {
      setProdError(errorMessage(err))
    } finally {
      setProdBusy(false)
    }
  }

  const toggleProductoActivo = async (p: ProductoDetalle) => {
    try {
      await apiPut(`/api/productos/${p.id}`, { activo: !p.activo })
      await loadProductos()
    } catch (err) {
      setProdError(errorMessage(err))
    }
  }

  // Depósito
  const [depForm, setDepForm] = useState({ nombre: '', ubicacion: '' })
  const [depEditId, setDepEditId] = useState<number | null>(null)
  const [depBusy, setDepBusy] = useState(false)
  const [depError, setDepError] = useState<string | null>(null)

  const nuevoDeposito = () => {
    setDepEditId(null)
    setDepForm({ nombre: '', ubicacion: '' })
    setDepError(null)
  }

  const editarDeposito = (d: DepositoDetalle) => {
    setDepEditId(d.id)
    setDepForm({ nombre: d.nombre, ubicacion: d.ubicacion || '' })
    setDepError(null)
  }

  const guardarDeposito = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!depForm.nombre.trim()) {
      setDepError('Nombre obligatorio.')
      return
    }
    setDepBusy(true)
    setDepError(null)
    try {
      if (depEditId === null) {
        await apiPost('/api/depositos', { nombre: depForm.nombre.trim(), ubicacion: depForm.ubicacion.trim() || undefined })
      } else {
        await apiPut(`/api/depositos/${depEditId}`, { nombre: depForm.nombre.trim(), ubicacion: depForm.ubicacion.trim() || undefined })
      }
      await loadDepositos()
      setDepEditId(null)
      setDepForm({ nombre: '', ubicacion: '' })
    } catch (err) {
      setDepError(errorMessage(err))
    } finally {
      setDepBusy(false)
    }
  }

  const toggleDepositoActivo = async (d: DepositoDetalle) => {
    try {
      await apiPut(`/api/depositos/${d.id}`, { activo: !d.activo })
      await loadDepositos()
    } catch (err) {
      setDepError(errorMessage(err))
    }
  }

  // Movimiento
  const [movForm, setMovForm] = useState({ producto_id: '', deposito_id: '', tipo: 'entrada', cantidad: '', sentido: 'sube', motivo: '', confirmar_stock_negativo: false })
  const [movBusy2, setMovBusy2] = useState(false)
  const [movError2, setMovError2] = useState<string | null>(null)
  const [confirmNeg, setConfirmNeg] = useState<{ open: boolean; payload: any } | null>(null)

  const enviarMovimiento = async (payloadBase: any, confirmar = false) => {
    setMovBusy2(true)
    setMovError2(null)
    try {
      const payload = { ...payloadBase, confirmar_stock_negativo: confirmar }
      const res = await apiPost<MovimientoResultado>('/api/inventario/movimientos', payload)
      await loadStock()
      await loadMovs()
      await loadBajoStock()
      return res
    } catch (err) {
      const msg = errorMessage(err)
      if ((err as ApiError).code === 'stock_insuficiente' || msg.toLowerCase().includes('negativo')) {
        setConfirmNeg({ open: true, payload: payloadBase })
        throw err
      }
      throw err
    } finally {
      setMovBusy2(false)
    }
  }

  const registrarMovimiento = async (e: React.FormEvent) => {
    e.preventDefault()
    setMovError2(null)
    const cantidad = parseMontoPositivo(movForm.cantidad)
    if (cantidad === null) {
      setMovError2('Cantidad debe ser un entero positivo.')
      return
    }
    if (!movForm.producto_id || !movForm.deposito_id) {
      setMovError2('Producto y depósito son obligatorios.')
      return
    }
    if (movForm.tipo === 'ajuste' && !movForm.sentido) {
      setMovError2('Sentido es obligatorio en ajuste.')
      return
    }
    if (movForm.tipo === 'ajuste' && !movForm.motivo.trim()) {
      setMovError2('Motivo obligatorio en ajuste.')
      return
    }
    const payload: any = {
      producto_id: Number(movForm.producto_id),
      deposito_id: Number(movForm.deposito_id),
      tipo: movForm.tipo,
      cantidad,
    }
    if (movForm.tipo === 'ajuste') {
      payload.sentido = movForm.sentido
      payload.motivo = movForm.motivo.trim()
    } else if (movForm.motivo.trim()) {
      payload.motivo = movForm.motivo.trim()
    }
    try {
      await enviarMovimiento(payload, false)
      setMovForm({ ...movForm, cantidad: '', motivo: '' })
      setConfirmNeg(null)
    } catch (err) {
      if (!confirmNeg) {
        setMovError2(errorMessage(err))
      }
    }
  }

  // Traslado
  const [trasForm, setTrasForm] = useState({ producto_id: '', deposito_origen_id: '', deposito_destino_id: '', cantidad: '', motivo: '' })
  const [trasBusy, setTrasBusy] = useState(false)
  const [trasError, setTrasError] = useState<string | null>(null)
  const [confirmTras, setConfirmTras] = useState<any | null>(null)

  const registrarTraslado = async (e: React.FormEvent) => {
    e.preventDefault()
    setTrasError(null)
    const cantidad = parseMontoPositivo(trasForm.cantidad)
    if (cantidad === null) {
      setTrasError('Cantidad debe ser un entero positivo.')
      return
    }
    if (!trasForm.producto_id || !trasForm.deposito_origen_id || !trasForm.deposito_destino_id) {
      setTrasError('Todos los campos son obligatorios.')
      return
    }
    if (trasForm.deposito_origen_id === trasForm.deposito_destino_id) {
      setTrasError('Origen y destino deben ser distintos.')
      return
    }
    const payload: any = {
      producto_id: Number(trasForm.producto_id),
      deposito_origen_id: Number(trasForm.deposito_origen_id),
      deposito_destino_id: Number(trasForm.deposito_destino_id),
      cantidad,
      motivo: trasForm.motivo.trim() || undefined,
    }
    setTrasBusy(true)
    try {
      await apiPost<TrasladoResultado>('/api/inventario/traslados', { ...payload, confirmar_stock_negativo: false })
      await loadStock()
      await loadMovs()
      await loadBajoStock()
      setTrasForm({ producto_id: '', deposito_origen_id: '', deposito_destino_id: '', cantidad: '', motivo: '' })
      setConfirmTras(null)
    } catch (err) {
      const msg = errorMessage(err)
      if ((err as ApiError).code === 'stock_insuficiente' || msg.toLowerCase().includes('negativo')) {
        setConfirmTras(payload)
      } else {
        setTrasError(msg)
      }
    } finally {
      setTrasBusy(false)
    }
  }

  const confirmarTraslado = async () => {
    if (!confirmTras) return
    setTrasBusy(true)
    setTrasError(null)
    try {
      await apiPost<TrasladoResultado>('/api/inventario/traslados', { ...confirmTras, confirmar_stock_negativo: true })
      await loadStock()
      await loadMovs()
      await loadBajoStock()
      setTrasForm({ producto_id: '', deposito_origen_id: '', deposito_destino_id: '', cantidad: '', motivo: '' })
      setConfirmTras(null)
    } catch (err) {
      setTrasError(errorMessage(err))
    } finally {
      setTrasBusy(false)
    }
  }

  const depositosActivos = useMemo(() => depositos.filter((d) => d.activo), [depositos])

  return (
    <AppShell>
      <section className="page">
        <header className="page-header">
          <div>
            <h1>Inventario</h1>
            <p className="muted">Productos, depósitos, stock, movimientos, traslados y bajo stock.</p>
          </div>
        </header>

        <div className="tabs">
          <button className={tab === 'productos' ? 'tab active' : 'tab'} onClick={() => setTab('productos')}>Productos</button>
          <button className={tab === 'depositos' ? 'tab active' : 'tab'} onClick={() => setTab('depositos')}>Depósitos</button>
          <button className={tab === 'stock' ? 'tab active' : 'tab'} onClick={() => setTab('stock')}>Stock</button>
          <button className={tab === 'movimientos' ? 'tab active' : 'tab'} onClick={() => setTab('movimientos')}>Movimientos</button>
          <button className={tab === 'traslados' ? 'tab active' : 'tab'} onClick={() => setTab('traslados')}>Traslados</button>
          <button className={tab === 'bajostock' ? 'tab active' : 'tab'} onClick={() => setTab('bajostock')}>Bajo stock</button>
        </div>

        {tab === 'productos' ? (
          <div className="grid-two">
            <article className="card">
              <header className="section-header">
                <h2>{prodEditId ? 'Editar producto' : 'Nuevo producto'}</h2>
                <button className="btn btn-small" type="button" onClick={abrirNuevoProducto}>Nuevo</button>
              </header>
              <form onSubmit={(e) => void guardarProducto(e)} className="stack">
                <div className="form-row">
                  <div className="field">
                    <label htmlFor="prod-codigo">Código</label>
                    <input id="prod-codigo" value={prodForm.codigo} onChange={(e) => setProdForm({ ...prodForm, codigo: e.target.value })} required />
                  </div>
                  <div className="field">
                    <label htmlFor="prod-nombre">Nombre</label>
                    <input id="prod-nombre" value={prodForm.nombre} onChange={(e) => setProdForm({ ...prodForm, nombre: e.target.value })} required />
                  </div>
                </div>
                <div className="form-row">
                  <div className="field">
                    <label htmlFor="prod-unidad">Unidad</label>
                    <input id="prod-unidad" value={prodForm.unidad} onChange={(e) => setProdForm({ ...prodForm, unidad: e.target.value })} />
                  </div>
                  <div className="field">
                    <label htmlFor="prod-stockmin">Stock mínimo</label>
                    <input id="prod-stockmin" inputMode="numeric" value={prodForm.stock_minimo} onChange={(e) => setProdForm({ ...prodForm, stock_minimo: e.target.value })} />
                  </div>
                  <div className="field">
                    <label htmlFor="prod-precio">Precio venta (Bs)</label>
                    <input id="prod-precio" inputMode="decimal" value={prodForm.precio_venta_bs} onChange={(e) => setProdForm({ ...prodForm, precio_venta_bs: e.target.value })} />
                  </div>
                </div>
                {prodError ? <p className="form-error" role="alert">{prodError}</p> : null}
                {prodMsg ? <p className="ok-text">{prodMsg}</p> : null}
                <div className="form-actions">
                  <button className="btn btn-primary" type="submit" disabled={prodBusy}>{prodBusy ? 'Guardando…' : 'Guardar'}</button>
                </div>
                <p className="muted small">No hay borrado. El stock inicial se registra con una entrada en Movimientos.</p>
              </form>
            </article>
            <article className="card">
              <header className="section-header">
                <h2>Catálogo</h2>
                <label className="inline-actions">
                  <input type="checkbox" checked={incluirInactivosProd} onChange={(e) => setIncluirInactivosProd(e.target.checked)} />
                  Incluir inactivos
                </label>
              </header>
              {productosLoading ? <p className="muted">Cargando…</p> : null}
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Código</th>
                      <th>Nombre</th>
                      <th>Unidad</th>
                      <th>Stock mín.</th>
                      <th>Precio</th>
                      <th>Estado</th>
                      <th>Acciones</th>
                    </tr>
                  </thead>
                  <tbody>
                    {productos.map((p) => (
                      <tr key={p.id}>
                        <td>{p.codigo}</td>
                        <td>{p.nombre}</td>
                        <td>{p.unidad || '—'}</td>
                        <td>{p.stock_minimo ?? 0}</td>
                        <td>{formatBs(p.precio_venta_bs ?? 0)}</td>
                        <td>{p.activo ? 'Activo' : 'Inactivo'}</td>
                        <td className="inline-actions">
                          <button className="btn btn-small" type="button" onClick={() => editarProducto(p)}>Editar</button>
                          <button className="btn btn-small" type="button" onClick={() => void toggleProductoActivo(p)}>{p.activo ? 'Desactivar' : 'Reactivar'}</button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {productos.length === 0 && !productosLoading ? <p className="muted">Sin productos.</p> : null}
            </article>
          </div>
        ) : null}

        {tab === 'depositos' ? (
          <div className="grid-two">
            <article className="card">
              <header className="section-header">
                <h2>{depEditId ? 'Editar depósito' : 'Nuevo depósito'}</h2>
                <button className="btn btn-small" type="button" onClick={nuevoDeposito}>Nuevo</button>
              </header>
              <form onSubmit={(e) => void guardarDeposito(e)} className="stack">
                <div className="field">
                  <label htmlFor="dep-nombre">Nombre</label>
                  <input id="dep-nombre" value={depForm.nombre} onChange={(e) => setDepForm({ ...depForm, nombre: e.target.value })} required />
                </div>
                <div className="field">
                  <label htmlFor="dep-ubicacion">Ubicación</label>
                  <input id="dep-ubicacion" value={depForm.ubicacion} onChange={(e) => setDepForm({ ...depForm, ubicacion: e.target.value })} />
                </div>
                {depError ? <p className="form-error" role="alert">{depError}</p> : null}
                <div className="form-actions">
                  <button className="btn btn-primary" type="submit" disabled={depBusy}>{depBusy ? 'Guardando…' : 'Guardar'}</button>
                </div>
                <p className="muted small">No hay borrado. Desactivar no borra ni oculta el stock histórico.</p>
              </form>
            </article>
            <article className="card">
              <header className="section-header">
                <h2>Depósitos</h2>
                <label className="inline-actions">
                  <input type="checkbox" checked={incluirInactivosDep} onChange={(e) => setIncluirInactivosDep(e.target.checked)} />
                  Incluir inactivos
                </label>
              </header>
              {depositosLoading ? <p className="muted">Cargando…</p> : null}
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Nombre</th>
                      <th>Ubicación</th>
                      <th>Estado</th>
                      <th>Acciones</th>
                    </tr>
                  </thead>
                  <tbody>
                    {depositos.map((d) => (
                      <tr key={d.id}>
                        <td>{d.nombre}</td>
                        <td>{d.ubicacion || '—'}</td>
                        <td>{d.activo ? 'Activo' : 'Inactivo'}</td>
                        <td className="inline-actions">
                          <button className="btn btn-small" type="button" onClick={() => editarDeposito(d)}>Editar</button>
                          <button className="btn btn-small" type="button" onClick={() => void toggleDepositoActivo(d)}>{d.activo ? 'Desactivar' : 'Reactivar'}</button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </article>
          </div>
        ) : null}

        {tab === 'stock' ? (
          <article className="card">
            <header className="section-header">
              <h2>Stock por producto/depósito</h2>
              <div className="inline-actions">
                <input placeholder="Producto ID" value={filtroProdStock} onChange={(e) => setFiltroProdStock(e.target.value)} />
                <input placeholder="Depósito ID" value={filtroDepStock} onChange={(e) => setFiltroDepStock(e.target.value)} />
                <button className="btn btn-small" type="button" onClick={() => void loadStock()}>Filtrar</button>
              </div>
            </header>
            {stockLoading ? <p className="muted">Cargando…</p> : null}
            {(stock ?? []).map((s) => (
              <div key={s.producto_id} className="stack">
                <h3>{s.producto_codigo} - {s.producto_nombre}</h3>
                <p className="muted">Stock total: {s.stock_total} | Mínimo: {s.stock_minimo}</p>
                <div className="table-wrap">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Depósito</th>
                        <th>Cantidad</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(s.depositos ?? []).map((d) => (
                        <tr key={`${s.producto_id}-${d.deposito_id}`}>
                          <td>{d.deposito_nombre}</td>
                          <td>{d.cantidad}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}
            {stock.length === 0 && !stockLoading ? <p className="muted">Sin stock.</p> : null}
          </article>
        ) : null}

        {tab === 'movimientos' ? (
          <div className="stack">
            <article className="card">
              <header className="section-header">
                <h2>Registrar movimiento manual</h2>
              </header>
              <form onSubmit={(e) => void registrarMovimiento(e)} className="stack">
                <div className="form-row">
                  <div className="field">
                    <label htmlFor="mv-prod">Producto</label>
                    <select id="mv-prod" value={movForm.producto_id} onChange={(e) => setMovForm({ ...movForm, producto_id: e.target.value })}>
                      <option value="">Seleccione</option>
                      {productos.map((p) => <option key={p.id} value={p.id}>{p.codigo} - {p.nombre}</option>)}
                    </select>
                  </div>
                  <div className="field">
                    <label htmlFor="mv-dep">Depósito</label>
                    <select id="mv-dep" value={movForm.deposito_id} onChange={(e) => setMovForm({ ...movForm, deposito_id: e.target.value })}>
                      <option value="">Seleccione</option>
                      {depositosActivos.map((d) => <option key={d.id} value={d.id}>{d.nombre}</option>)}
                    </select>
                  </div>
                  <div className="field">
                    <label htmlFor="mv-tipo">Tipo</label>
                    <select id="mv-tipo" value={movForm.tipo} onChange={(e) => setMovForm({ ...movForm, tipo: e.target.value })}>
                      <option value="entrada">entrada</option>
                      <option value="salida">salida</option>
                      <option value="ajuste">ajuste</option>
                    </select>
                  </div>
                </div>
                <div className="form-row">
                  {movForm.tipo === 'ajuste' ? (
                    <div className="field">
                      <label htmlFor="mv-sentido">Sentido</label>
                      <select id="mv-sentido" value={movForm.sentido} onChange={(e) => setMovForm({ ...movForm, sentido: e.target.value })}>
                        <option value="sube">sube</option>
                        <option value="baja">baja</option>
                      </select>
                    </div>
                  ) : null}
                  <div className="field">
                    <label htmlFor="mv-cant">Cantidad</label>
                    <input id="mv-cant" inputMode="numeric" value={movForm.cantidad} onChange={(e) => setMovForm({ ...movForm, cantidad: e.target.value })} />
                  </div>
                  <div className="field">
                    <label htmlFor="mv-motivo">Motivo {movForm.tipo === 'ajuste' ? '(obligatorio)' : '(opcional)'}</label>
                    <input id="mv-motivo" value={movForm.motivo} onChange={(e) => setMovForm({ ...movForm, motivo: e.target.value })} />
                  </div>
                </div>
                {movError2 ? <p className="form-error" role="alert">{movError2}</p> : null}
                <div className="form-actions">
                  <button className="btn btn-primary" type="submit" disabled={movBusy2}>{movBusy2 ? 'Registrando…' : 'Registrar'}</button>
                </div>
              </form>
            </article>

            <article className="card">
              <header className="section-header">
                <h2>Historial de movimientos</h2>
                <div className="inline-actions">
                  <input placeholder="Producto ID" value={filtroProdMov} onChange={(e) => setFiltroProdMov(e.target.value)} />
                  <input placeholder="Depósito ID" value={filtroDepMov} onChange={(e) => setFiltroDepMov(e.target.value)} />
                  <button className="btn btn-small" type="button" onClick={() => void loadMovs()}>Filtrar</button>
                </div>
              </header>
              {movsLoading ? <p className="muted">Cargando…</p> : null}
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Tipo</th>
                      <th>Producto</th>
                      <th>Depósito</th>
                      <th>Cantidad</th>
                      <th>Motivo</th>
                      <th>Usuario</th>
                      <th>Fecha</th>
                    </tr>
                  </thead>
                  <tbody>
                    {movs.slice(0, 200).map((m) => (
                      <tr key={m.id}>
                        <td>{m.tipo}</td>
                        <td>{m.producto_codigo || m.producto_id} {m.producto_nombre ? `- ${m.producto_nombre}` : ''}</td>
                        <td>{m.deposito_nombre || m.deposito_id}</td>
                        <td>{m.cantidad}</td>
                        <td>{m.motivo || '—'}</td>
                        <td>{usuarioNombre(m.usuario_id)}</td>
                        <td>{new Date(m.creado_en).toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {movs.length === 0 && !movsLoading ? <p className="muted">Sin movimientos.</p> : null}
            </article>
          </div>
        ) : null}

        {tab === 'traslados' ? (
          <article className="card">
            <header className="section-header">
              <h2>Traslado entre depósitos</h2>
            </header>
            <form onSubmit={(e) => void registrarTraslado(e)} className="stack">
              <div className="form-row">
                <div className="field">
                  <label htmlFor="tr-prod">Producto</label>
                  <select id="tr-prod" value={trasForm.producto_id} onChange={(e) => setTrasForm({ ...trasForm, producto_id: e.target.value })}>
                    <option value="">Seleccione</option>
                    {productos.map((p) => <option key={p.id} value={p.id}>{p.codigo} - {p.nombre}</option>)}
                  </select>
                </div>
                <div className="field">
                  <label htmlFor="tr-orig">Depósito origen</label>
                  <select id="tr-orig" value={trasForm.deposito_origen_id} onChange={(e) => setTrasForm({ ...trasForm, deposito_origen_id: e.target.value })}>
                    <option value="">Seleccione</option>
                    {depositosActivos.map((d) => <option key={d.id} value={d.id}>{d.nombre}</option>)}
                  </select>
                </div>
                <div className="field">
                  <label htmlFor="tr-dest">Depósito destino</label>
                  <select id="tr-dest" value={trasForm.deposito_destino_id} onChange={(e) => setTrasForm({ ...trasForm, deposito_destino_id: e.target.value })}>
                    <option value="">Seleccione</option>
                    {depositosActivos.map((d) => <option key={d.id} value={d.id}>{d.nombre}</option>)}
                  </select>
                </div>
              </div>
              <div className="form-row">
                <div className="field">
                  <label htmlFor="tr-cant">Cantidad</label>
                  <input id="tr-cant" inputMode="numeric" value={trasForm.cantidad} onChange={(e) => setTrasForm({ ...trasForm, cantidad: e.target.value })} />
                </div>
                <div className="field">
                  <label htmlFor="tr-motivo">Motivo (opcional)</label>
                  <input id="tr-motivo" value={trasForm.motivo} onChange={(e) => setTrasForm({ ...trasForm, motivo: e.target.value })} />
                </div>
              </div>
              {trasError ? <p className="form-error" role="alert">{trasError}</p> : null}
              <div className="form-actions">
                <button className="btn btn-primary" type="submit" disabled={trasBusy}>{trasBusy ? 'Trasladando…' : 'Trasladar'}</button>
              </div>
            </form>
          </article>
        ) : null}

        {tab === 'bajostock' ? (
          <article className="card">
            <header className="section-header">
              <h2>Bajo stock agrupado por proveedor</h2>
            </header>
            {bajoStockLoading ? <p className="muted">Cargando…</p> : null}
            {(bajoStock ?? []).map((g, idx) => (
              <div key={idx} className="stack">
                <h3>{g.proveedor_nombre || 'Sin proveedor'}</h3>
                <div className="table-wrap">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Código</th>
                        <th>Nombre</th>
                        <th>Mínimo</th>
                        <th>Total</th>
                        <th>Por depósito</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(g.productos ?? []).map((p) => (
                        <tr key={p.producto_id}>
                          <td>{p.codigo}</td>
                          <td>{p.nombre}</td>
                          <td>{p.stock_minimo}</td>
                          <td>{p.stock_total}</td>
                          <td>{(p.depositos ?? []).map((d) => `${d.deposito_nombre}: ${d.cantidad}`).join(', ') || '—'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}
            {bajoStock.length === 0 && !bajoStockLoading ? <p className="muted">Sin productos bajo stock.</p> : null}
          </article>
        ) : null}
      </section>

      {confirmNeg?.open ? (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal">
            <h3>Stock negativo</h3>
            <p>El resultado quedará en stock negativo. ¿Confirmar el movimiento?</p>
            <div className="modal-actions">
              <button className="btn" type="button" onClick={() => setConfirmNeg(null)}>Cancelar</button>
              <button
                className="btn btn-primary"
                type="button"
                onClick={async () => {
                  try {
                    await enviarMovimiento(confirmNeg.payload, true)
                    setConfirmNeg(null)
                    setMovForm({ ...movForm, cantidad: '', motivo: '' })
                  } catch (err) {
                    setMovError2(errorMessage(err))
                    setConfirmNeg(null)
                  }
                }}
              >
                Confirmar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {confirmTras ? (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal">
            <h3>Stock negativo en origen</h3>
            <p>El depósito origen quedará en stock negativo. ¿Confirmar el traslado?</p>
            <div className="modal-actions">
              <button className="btn" type="button" onClick={() => setConfirmTras(null)}>Cancelar</button>
              <button className="btn btn-primary" type="button" onClick={() => void confirmarTraslado()}>Confirmar</button>
            </div>
          </div>
        </div>
      ) : null}
    </AppShell>
  )
}
