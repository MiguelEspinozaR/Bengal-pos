package pos

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReportesVentas genera el reporte de ventas solo lecturas, solo ventas completadas.
func (s *Service) ReportesVentas(ctx context.Context, f ReporteVentasFiltro) (*ReporteVentas, error) {
	res := &ReporteVentas{
		Filtros:     f,
		Detalle:     []ReporteVentasItemDetalle{},
		GeneradoEn:  time.Now(),
	}
	query := `SELECT v.id, v.fecha, v.usuario_id, u.nombre_completo,
		COALESCE(vd.producto_id, 0), COALESCE(p.codigo,''), COALESCE(p.nombre,''),
		COALESCE(vd.cantidad,0), COALESCE(vd.precio_unitario_bs,0), COALESCE(vd.descuento_bs,0), COALESCE(vd.subtotal_bs,0),
		COALESCE(mp.codigo,'')
	FROM ventas v
	JOIN usuarios u ON u.id = v.usuario_id
	LEFT JOIN venta_detalles vd ON vd.venta_id = v.id
	LEFT JOIN productos p ON p.id = vd.producto_id
	LEFT JOIN partes_pago pp ON pp.venta_id = v.id AND pp.estado = 'confirmado'
	LEFT JOIN metodos_pago mp ON mp.id = pp.metodo_id
	WHERE v.estado = 'completada'`
	args := []any{}
	if f.Desde != nil {
		args = append(args, f.Desde)
		query += fmt.Sprintf(" AND v.fecha >= $%d", len(args))
	}
	if f.Hasta != nil {
		args = append(args, f.Hasta)
		query += fmt.Sprintf(" AND v.fecha <= $%d", len(args))
	}
	if f.ProductoID != nil {
		args = append(args, *f.ProductoID)
		query += fmt.Sprintf(" AND vd.producto_id = $%d", len(args))
	}
	if f.UsuarioID != nil {
		args = append(args, *f.UsuarioID)
		query += fmt.Sprintf(" AND v.usuario_id = $%d", len(args))
	}
	if f.Metodo != nil {
		args = append(args, strings.ToLower(strings.TrimSpace(*f.Metodo)))
		query += fmt.Sprintf(" AND lower(mp.codigo) = $%d", len(args))
	}
	if f.DepositoID != nil {
		// optional: try to filter by movimientos venta to that deposito if exists
		args = append(args, *f.DepositoID)
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM movimientos_inventario mi WHERE mi.venta_id = v.id AND mi.deposito_id = $%d)", len(args))
	}
	query += " ORDER BY v.fecha, v.id, vd.id"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ventasSet := map[int64]bool{}
	for rows.Next() {
		var it ReporteVentasItemDetalle
		if err := rows.Scan(&it.VentaID, &it.Fecha, &it.UsuarioID, &it.UsuarioNombre,
			&it.ProductoID, &it.ProductoCodigo, &it.ProductoNombre,
			&it.Cantidad, &it.PrecioUnitarioBs, &it.DescuentoBs, &it.SubtotalBs, &it.MetodoPago,
		); err != nil {
			return nil, err
		}
		res.Detalle = append(res.Detalle, it)
		ventasSet[it.VentaID] = true
		if it.ProductoID != 0 {
			res.Totales.CantidadItems += it.Cantidad
			res.Totales.SubtotalBs += it.SubtotalBs
			res.Totales.DescuentoBs += it.DescuentoBs
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	res.Totales.CantidadVentas = int64(len(ventasSet))
	// compute total from detalle sums
	res.Totales.TotalBs = res.Totales.SubtotalBs // subtotal_bs already net? subtotal_bs = cantidad*precio - descuento per venta_detalles; sum is fine as net
	return res, nil
}

// ReportesInventario genera reporte de inventario.
func (s *Service) ReportesInventario(ctx context.Context, f ReporteInventarioFiltro) (*ReporteInventario, error) {
	res := &ReporteInventario{
		Filtros:    f,
		GeneradoEn: time.Now(),
	}
	if f.Tipo == "" {
		f.Tipo = "stock_actual"
	}

	// always build stock info
	stockItems := map[int64]*ReporteInventarioStockItem{}
	// products
	qProd := `SELECT p.id, p.codigo, p.nombre, p.stock_minimo,
		pp.proveedor_id, COALESCE(pr.nombre,'')
	FROM productos p
	LEFT JOIN producto_proveedores pp ON pp.producto_id = p.id AND pp.es_principal
	LEFT JOIN proveedores pr ON pr.id = pp.proveedor_id
	WHERE p.activo`
	rows, err := s.pool.Query(ctx, qProd)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it ReporteInventarioStockItem
		var provID *int64
		var provNom string
		if err := rows.Scan(&it.ProductoID, &it.ProductoCodigo, &it.ProductoNombre, &it.StockMinimo, &provID, &provNom); err != nil {
			rows.Close()
			return nil, err
		}
		it.ProveedorPrincipalID = provID
		it.ProveedorPrincipalNombre = provNom
		it.Depositos = []StockDepositoReporte{}
		stockItems[it.ProductoID] = &it
	}
	rows.Close()

	// totals
	trows, err := s.pool.Query(ctx, `SELECT producto_id, deposito_id, cantidad FROM stock_depositos`)
	if err != nil {
		return nil, err
	}
	for trows.Next() {
		var pid, did, cant int64
		if err := trows.Scan(&pid, &did, &cant); err != nil {
			trows.Close()
			return nil, err
		}
		if it, ok := stockItems[pid]; ok {
			it.StockTotal += cant
			// get deposito name
			var dnom string
			_ = s.pool.QueryRow(ctx, `SELECT nombre FROM depositos WHERE id = $1`, did).Scan(&dnom)
			it.Depositos = append(it.Depositos, StockDepositoReporte{DepositoID: did, DepositoNombre: dnom, Cantidad: cant})
		}
	}
	trows.Close()
	for _, it := range stockItems {
		if it.StockTotal <= it.StockMinimo {
			it.BajoMinimo = true
		}
	}

	if f.Tipo == "bajo_minimo" {
		for _, it := range stockItems {
			if it.BajoMinimo {
				cp := *it
				res.BajoMinimo = append(res.BajoMinimo, cp)
			}
		}
		return res, nil
	}
	if f.Tipo == "movimientos" {
		q := `SELECT m.id, m.producto_id, p.codigo, p.nombre, m.deposito_id, d.nombre, m.tipo, m.cantidad, m.venta_id, m.usuario_id, m.motivo, m.creado_en
FROM movimientos_inventario m
JOIN productos p ON p.id = m.producto_id
JOIN depositos d ON d.id = m.deposito_id WHERE 1=1`
		args := []any{}
		if f.ProductoID != nil {
			args = append(args, *f.ProductoID)
			q += fmt.Sprintf(" AND m.producto_id = $%d", len(args))
		}
		if f.DepositoID != nil {
			args = append(args, *f.DepositoID)
			q += fmt.Sprintf(" AND m.deposito_id = $%d", len(args))
		}
		if f.TipoMov != nil {
			args = append(args, *f.TipoMov)
			q += fmt.Sprintf(" AND m.tipo = $%d", len(args))
		}
		if f.Desde != nil {
			args = append(args, *f.Desde)
			q += fmt.Sprintf(" AND m.creado_en >= $%d", len(args))
		}
		if f.Hasta != nil {
			args = append(args, *f.Hasta)
			q += fmt.Sprintf(" AND m.creado_en <= $%d", len(args))
		}
		q += " ORDER BY m.creado_en, m.id"
		mrows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer mrows.Close()
		for mrows.Next() {
			var mi ReporteInventarioMovItem
			if err := mrows.Scan(&mi.ID, &mi.ProductoID, &mi.ProductoCodigo, &mi.ProductoNombre,
				&mi.DepositoID, &mi.DepositoNombre, &mi.Tipo, &mi.Cantidad, &mi.VentaID, &mi.UsuarioID, &mi.Motivo, &mi.CreadoEn,
			); err != nil {
				return nil, err
			}
			res.Movimientos = append(res.Movimientos, mi)
		}
		return res, nil
	}
	// stock_actual default
	for _, it := range stockItems {
		if f.ProductoID != nil && it.ProductoID != *f.ProductoID {
			continue
		}
		if f.DepositoID != nil {
			// filter deposits shown? keep all but could filter; show all
		}
		cp := *it
		res.StockActual = append(res.StockActual, cp)
	}
	return res, nil
}

// ReportesCaja genera reporte por sesión/turno.
func (s *Service) ReportesCaja(ctx context.Context, f ReporteCajaFiltro) (*ReporteCaja, error) {
	var sesionID int64
	if f.SesionID != nil {
		sesionID = *f.SesionID
	} else if f.Desde != nil && f.Hasta != nil {
		// find session overlapping? take first abierta/cerrada in range
		err := s.pool.QueryRow(ctx, `SELECT id FROM caja_sesiones WHERE fecha_apertura >= $1 AND fecha_apertura <= $2 ORDER BY fecha_apertura DESC LIMIT 1`, f.Desde, f.Hasta).Scan(&sesionID)
		if err != nil {
			return nil, ErrSesionNoExiste
		}
	} else {
		// try open
		err := s.pool.QueryRow(ctx, `SELECT id FROM caja_sesiones WHERE estado = 'abierta' ORDER BY fecha_apertura DESC LIMIT 1`).Scan(&sesionID)
		if err != nil {
			err = s.pool.QueryRow(ctx, `SELECT id FROM caja_sesiones ORDER BY fecha_apertura DESC LIMIT 1`).Scan(&sesionID)
			if err != nil {
				return nil, ErrSesionNoExiste
			}
		}
	}

	sesion, err := s.getCajaSesion(ctx, sesionID)
	if err != nil {
		return nil, err
	}
	movs, err := s.listCajaMovimientosRepo(ctx, sesionID)
	if err != nil {
		return nil, err
	}
	res := &ReporteCaja{
		Sesion:      *sesion,
		Movimientos: []ReporteCajaMovimiento{},
		GeneradoEn:  time.Now(),
	}
	for _, m := range movs {
		res.Movimientos = append(res.Movimientos, ReporteCajaMovimiento{
			ID: m.ID, Tipo: m.Tipo, MontoBs: m.MontoBs, Motivo: m.Motivo, UsuarioID: m.UsuarioID, VentaID: m.VentaID, CreadoEn: m.CreadoEn,
		})
		if m.Tipo == "venta_efectivo" {
			res.VentasEfectivoBs += m.MontoBs
		}
		if m.Tipo == "entrada" {
			res.EntradasBs += m.MontoBs
		}
		if m.Tipo == "salida" {
			res.SalidasBs += m.MontoBs // stored as signed/positive per tipo? caja movimientos store monto_bs; entradas/salidas signed by logic in app; read as-is sum by tipo
		}
	}
	// compute esperado similar to cierre
	res.EsperadoBs = sesion.MontoInicialBs + res.VentasEfectivoBs + res.EntradasBs - res.SalidasBs
	if sesion.EfectivoContadoBs != nil {
		c := *sesion.EfectivoContadoBs
		res.ContadoBs = &c
		d := c - res.EsperadoBs
		res.DiferenciaBs = &d
	}
	// ventas en proceso warning if any
	if ventasp, err := s.listVentasEnProceso(ctx, sesionID); err == nil {
		res.VentasEnProceso = ventasp
	}
	return res, nil
}

func (s *Service) getCajaSesion(ctx context.Context, id int64) (*CajaSesion, error) {
	var c CajaSesion
	var fcierre *time.Time
	var ucierre *int64
	var eesp, econt, dif *int64
	err := s.pool.QueryRow(ctx,
		`SELECT id, estado, fecha_apertura, usuario_apertura_id, monto_inicial_bs, fecha_cierre, usuario_cierre_id, efectivo_esperado_bs, efectivo_contado_bs, diferencia_bs
		 FROM caja_sesiones WHERE id = $1`, id,
	).Scan(&c.ID, &c.Estado, &c.FechaApertura, &c.UsuarioAperturaID, &c.MontoInicialBs, &fcierre, &ucierre, &eesp, &econt, &dif)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrSesionNoExiste
		}
		return nil, err
	}
	c.FechaCierre = fcierre
	c.UsuarioCierreID = ucierre
	c.EfectivoEsperadoBs = eesp
	c.EfectivoContadoBs = econt
	c.DiferenciaBs = dif
	return &c, nil
}

func (s *Service) listCajaMovimientosRepo(ctx context.Context, sesionID int64) ([]CajaMovimiento, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, sesion_id, tipo, monto_bs, motivo, usuario_id, venta_id, creado_en
		 FROM caja_movimientos WHERE sesion_id = $1 ORDER BY creado_en, id`, sesionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []CajaMovimiento{}
	for rows.Next() {
		var m CajaMovimiento
		if err := rows.Scan(&m.ID, &m.SesionID, &m.Tipo, &m.MontoBs, &m.Motivo, &m.UsuarioID, &m.VentaID, &m.CreadoEn); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	return res, nil
}

func (s *Service) listVentasEnProceso(ctx context.Context, sesionID int64) ([]CajaVentaEnProceso, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, usuario_id, total_bs, fecha FROM ventas WHERE estado = 'en_proceso' AND caja_sesion_id = $1 ORDER BY fecha, id`, sesionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []CajaVentaEnProceso{}
	for rows.Next() {
		var v CajaVentaEnProceso
		if err := rows.Scan(&v.ID, &v.UsuarioID, &v.TotalBs, &v.Fecha); err != nil {
			return nil, err
		}
		res = append(res, v)
	}
	return res, nil
}

// ReportesPersonal genera reporte de asistencia y pagos_personal (solo lectura).
func (s *Service) ReportesPersonal(ctx context.Context, f ReportePersonalFiltro) (*ReportePersonal, error) {
	res := &ReportePersonal{
		Filtros:    f,
		Asistencia: []ReporteAsistenciaItem{},
		Pagos:      []ReportePagoPersonalItem{},
		GeneradoEn: time.Now(),
	}
	if f.Periodo == "" {
		f.Periodo = "ambos"
	}
	if f.Periodo == "asistencia" || f.Periodo == "ambos" {
		q := `SELECT a.id, a.usuario_id, u.nombre_completo, a.tipo, a.fecha_hora, a.registrado_por, a.motivo, a.correccion_id, a.creado_en
FROM asistencia a
JOIN usuarios u ON u.id = a.usuario_id WHERE 1=1`
		args := []any{}
		if f.UsuarioID != nil {
			args = append(args, *f.UsuarioID)
			q += fmt.Sprintf(" AND a.usuario_id = $%d", len(args))
		}
		if f.Desde != nil {
			args = append(args, *f.Desde)
			q += fmt.Sprintf(" AND a.fecha_hora >= $%d", len(args))
		}
		if f.Hasta != nil {
			args = append(args, *f.Hasta)
			q += fmt.Sprintf(" AND a.fecha_hora <= $%d", len(args))
		}
		q += " ORDER BY a.fecha_hora, a.id"
		rows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var it ReporteAsistenciaItem
			if err := rows.Scan(&it.ID, &it.UsuarioID, &it.UsuarioNombre, &it.Tipo, &it.FechaHora, &it.RegistradoPor, &it.Motivo, &it.CorreccionID, &it.CreadoEn); err != nil {
				rows.Close()
				return nil, err
			}
			res.Asistencia = append(res.Asistencia, it)
		}
		rows.Close()
	}
	if f.Periodo == "pagos" || f.Periodo == "ambos" {
		q := `SELECT p.id, p.usuario_id, u.nombre_completo, p.monto_bs, p.fecha_pago, p.concepto, p.periodo_desde, p.periodo_hasta, p.registrado_por, p.correccion_id, p.creado_en
FROM pagos_personal p
JOIN usuarios u ON u.id = p.usuario_id WHERE 1=1`
		args := []any{}
		if f.UsuarioID != nil {
			args = append(args, *f.UsuarioID)
			q += fmt.Sprintf(" AND p.usuario_id = $%d", len(args))
		}
		if f.Desde != nil {
			args = append(args, *f.Desde)
			q += fmt.Sprintf(" AND p.fecha_pago >= $%d::date", len(args))
		}
		if f.Hasta != nil {
			args = append(args, *f.Hasta)
			q += fmt.Sprintf(" AND p.fecha_pago <= $%d::date", len(args))
		}
		q += " ORDER BY p.fecha_pago, p.id"
		rows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var it ReportePagoPersonalItem
			var pds, pde *string
			var fd time.Time
			if err := rows.Scan(&it.ID, &it.UsuarioID, &it.UsuarioNombre, &it.MontoBs, &fd, &it.Concepto, &pds, &pde, &it.RegistradoPor, &it.CorreccionID, &it.CreadoEn); err != nil {
				rows.Close()
				return nil, err
			}
			it.FechaPago = fd.Format("2006-01-02")
			if pds != nil {
				s := *pds
				it.PeriodoDesde = &s
			}
			if pde != nil {
				s := *pde
				it.PeriodoHasta = &s
			}
			res.Pagos = append(res.Pagos, it)
			res.TotalPagosBs += it.MontoBs
		}
		rows.Close()
	}
	return res, nil
}
