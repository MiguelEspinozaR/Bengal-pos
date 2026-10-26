package pos

import "time"

// ReporteVentasFiltro define filtros para reporte de ventas.
type ReporteVentasFiltro struct {
	Desde      *time.Time
	Hasta      *time.Time
	ProductoID *int64
	UsuarioID  *int64
	Metodo     *string
	DepositoID *int64
}

// ReporteVentasItemDetalle representa un renglón de detalle de venta.
type ReporteVentasItemDetalle struct {
	VentaID         int64     `json:"venta_id"`
	Fecha           time.Time `json:"fecha"`
	UsuarioID       int64     `json:"usuario_id"`
	UsuarioNombre   string    `json:"usuario_nombre"`
	ProductoID      int64     `json:"producto_id"`
	ProductoCodigo  string    `json:"producto_codigo"`
	ProductoNombre  string    `json:"producto_nombre"`
	Cantidad        int64     `json:"cantidad"`
	PrecioUnitarioBs int64    `json:"precio_unitario_bs"`
	DescuentoBs     int64     `json:"descuento_bs"`
	SubtotalBs      int64     `json:"subtotal_bs"`
	MetodoPago      string    `json:"metodo_pago"`
}

// ReporteVentasTotales agrupa totales enteros en Bs.
type ReporteVentasTotales struct {
	CantidadVentas int64 `json:"cantidad_ventas"`
	CantidadItems  int64 `json:"cantidad_items"`
	SubtotalBs     int64 `json:"subtotal_bs"`
	DescuentoBs    int64 `json:"descuento_bs"`
	TotalBs        int64 `json:"total_bs"`
}

// ReporteVentas resultado completo para ventas.
type ReporteVentas struct {
	Filtros  ReporteVentasFiltro  `json:"filtros,omitempty"`
	Detalle  []ReporteVentasItemDetalle `json:"detalle"`
	Totales  ReporteVentasTotales `json:"totales"`
	GeneradoEn time.Time          `json:"generado_en"`
}

// ReporteInventarioFiltro define filtros para inventario.
type ReporteInventarioFiltro struct {
	Tipo        string // stock_actual | bajo_minimo | movimientos
	ProductoID  *int64
	DepositoID  *int64
	Desde       *time.Time
	Hasta       *time.Time
	TipoMov     *string // filtro por tipo de movimiento
}

// StockDepositoReporte detalle por depósito.
type StockDepositoReporte struct {
	DepositoID     int64  `json:"deposito_id"`
	DepositoNombre string `json:"deposito_nombre"`
	Cantidad       int64  `json:"cantidad"`
}

// ReporteInventarioStockItem stock actual por producto.
type ReporteInventarioStockItem struct {
	ProductoID       int64                   `json:"producto_id"`
	ProductoCodigo   string                  `json:"producto_codigo"`
	ProductoNombre   string                  `json:"producto_nombre"`
	StockMinimo      int64                   `json:"stock_minimo"`
	StockTotal       int64                   `json:"stock_total"`
	BajoMinimo       bool                    `json:"bajo_minimo"`
	ProveedorPrincipalID *int64              `json:"proveedor_principal_id,omitempty"`
	ProveedorPrincipalNombre string           `json:"proveedor_principal_nombre,omitempty"`
	Depositos        []StockDepositoReporte  `json:"depositos"`
}

// ReporteInventarioMovItem movimiento de inventario.
type ReporteInventarioMovItem struct {
	ID            int64     `json:"id"`
	ProductoID    int64     `json:"producto_id"`
	ProductoCodigo string   `json:"producto_codigo"`
	ProductoNombre string   `json:"producto_nombre"`
	DepositoID    int64     `json:"deposito_id"`
	DepositoNombre string   `json:"deposito_nombre"`
	Tipo          string    `json:"tipo"`
	Cantidad      int64     `json:"cantidad"`
	VentaID       *int64    `json:"venta_id,omitempty"`
	UsuarioID     int64     `json:"usuario_id"`
	Motivo        *string   `json:"motivo,omitempty"`
	CreadoEn      time.Time `json:"creado_en"`
}

// ReporteInventario resultado para inventario.
type ReporteInventario struct {
	Filtros     ReporteInventarioFiltro      `json:"filtros"`
	StockActual []ReporteInventarioStockItem `json:"stock_actual,omitempty"`
	BajoMinimo  []ReporteInventarioStockItem `json:"bajo_minimo,omitempty"`
	Movimientos []ReporteInventarioMovItem   `json:"movimientos,omitempty"`
	GeneradoEn  time.Time                    `json:"generado_en"`
}

// ReporteCajaFiltro filtros para caja.
type ReporteCajaFiltro struct {
	SesionID       *int64
	Desde          *time.Time
	Hasta          *time.Time
	UsuarioAperturaID *int64
}

// ReporteCajaMovimiento movimiento de caja en reporte.
type ReporteCajaMovimiento struct {
	ID        int64     `json:"id"`
	Tipo      string    `json:"tipo"`
	MontoBs   int64     `json:"monto_bs"`
	Motivo    *string   `json:"motivo,omitempty"`
	UsuarioID int64     `json:"usuario_id"`
	VentaID   *int64    `json:"venta_id,omitempty"`
	CreadoEn  time.Time `json:"creado_en"`
}

// ReporteCaja resultado por turno/sesión.
type ReporteCaja struct {
	Sesion        CajaSesion             `json:"sesion"`
	Movimientos   []ReporteCajaMovimiento `json:"movimientos"`
	VentasEfectivoBs int64               `json:"ventas_efectivo_bs"`
	EntradasBs      int64               `json:"entradas_bs"`
	SalidasBs       int64               `json:"salidas_bs"`
	EsperadoBs      int64               `json:"esperado_bs"`
	ContadoBs       *int64              `json:"contado_bs,omitempty"`
	DiferenciaBs    *int64              `json:"diferencia_bs,omitempty"`
	VentasEnProceso []CajaVentaEnProceso `json:"ventas_en_proceso,omitempty"`
	GeneradoEn      time.Time           `json:"generado_en"`
}

// ReportePersonalFiltro filtros para personal.
type ReportePersonalFiltro struct {
	UsuarioID *int64
	Desde     *time.Time
	Hasta     *time.Time
	Periodo   string // "asistencia" | "pagos" | "ambos"
}

// ReporteAsistenciaItem asistencia.
type ReporteAsistenciaItem struct {
	ID            int64     `json:"id"`
	UsuarioID     int64     `json:"usuario_id"`
	UsuarioNombre string    `json:"usuario_nombre"`
	Tipo          string    `json:"tipo"`
	FechaHora     time.Time `json:"fecha_hora"`
	RegistradoPor int64     `json:"registrado_por"`
	Motivo        *string   `json:"motivo,omitempty"`
	CorreccionID  *int64    `json:"correccion_id,omitempty"`
	CreadoEn      time.Time `json:"creado_en"`
}

// ReportePagoPersonalItem pago.
type ReportePagoPersonalItem struct {
	ID            int64     `json:"id"`
	UsuarioID     int64     `json:"usuario_id"`
	UsuarioNombre string    `json:"usuario_nombre"`
	MontoBs       int64     `json:"monto_bs"`
	FechaPago     string    `json:"fecha_pago"`
	Concepto      string    `json:"concepto"`
	PeriodoDesde  *string   `json:"periodo_desde,omitempty"`
	PeriodoHasta  *string   `json:"periodo_hasta,omitempty"`
	RegistradoPor int64     `json:"registrado_por"`
	CorreccionID  *int64    `json:"correccion_id,omitempty"`
	CreadoEn      time.Time `json:"creado_en"`
}

// ReportePersonal resultado para personal.
type ReportePersonal struct {
	Filtros     ReportePersonalFiltro    `json:"filtros"`
	Asistencia  []ReporteAsistenciaItem  `json:"asistencia,omitempty"`
	Pagos       []ReportePagoPersonalItem `json:"pagos,omitempty"`
	TotalPagosBs int64                  `json:"total_pagos_bs"`
	GeneradoEn  time.Time               `json:"generado_en"`
}
