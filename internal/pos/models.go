package pos

import "time"

// CreateSaleItem is an input line. The server seals the catalog price when the
// line is added (PG-04).
type CreateSaleItem struct {
	ProductoID int64 `json:"producto_id"`
	Cantidad   int64 `json:"cantidad"`
}

// CreateSaleInput creates an "en_proceso" sale.
type CreateSaleInput struct {
	UsuarioID          int64            `json:"usuario_id"`
	Items              []CreateSaleItem `json:"items"`
	DescuentoBs        int64            `json:"descuento_bs"`
	DescuentoUsuarioID *int64           `json:"descuento_usuario_id"`
}

// AddPaymentPartInput registers a pending payment part. Cash parts may include
// the received amount so the change can be returned immediately (PG-01).
type AddPaymentPartInput struct {
	UsuarioID    int64   `json:"usuario_id"`
	MetodoCodigo string  `json:"metodo"`
	MontoBs      int64   `json:"monto_bs"`
	RecibidoBs   *int64  `json:"recibido_bs"`
	Referencia   *string `json:"referencia"`
}

// CompleteSaleInput closes the sale atomically (RT-03).
type CompleteSaleInput struct {
	UsuarioID              int64
	IdempotencyKey         string
	DepositoID             *int64
	ConfirmarStockNegativo bool
}

// SaleLine is a persisted sale detail.
type SaleLine struct {
	ID               int64  `json:"id"`
	ProductoID       int64  `json:"producto_id"`
	ProductoCodigo   string `json:"producto_codigo,omitempty"`
	ProductoNombre   string `json:"producto_nombre,omitempty"`
	Cantidad         int64  `json:"cantidad"`
	PrecioUnitarioBs int64  `json:"precio_unitario_bs"`
	DescuentoBs      int64  `json:"descuento_bs"`
	SubtotalBs       int64  `json:"subtotal_bs"`
}

// PaymentPart is one manually confirmed part of a split payment.
type PaymentPart struct {
	ID            int64      `json:"id"`
	VentaID       int64      `json:"venta_id"`
	MetodoCodigo  string     `json:"metodo"`
	MetodoNombre  string     `json:"metodo_nombre,omitempty"`
	MontoBs       int64      `json:"monto_bs"`
	Estado        string     `json:"estado"`
	ConfirmadoPor *int64     `json:"confirmado_por,omitempty"`
	ConfirmadoEn  *time.Time `json:"confirmado_en,omitempty"`
	Referencia    *string    `json:"referencia,omitempty"`
	CreadoEn      time.Time  `json:"creado_en"`
	CambioBs      int64      `json:"cambio_bs"`
}

// Sale is the persisted sale aggregate.
type Sale struct {
	ID           int64         `json:"id"`
	Estado       string        `json:"estado"`
	UsuarioID    int64         `json:"usuario_id"`
	CajaSesionID *int64        `json:"caja_sesion_id,omitempty"`
	Fecha        time.Time     `json:"fecha"`
	TotalBs      int64         `json:"total_bs"`
	DescuentoBs  int64         `json:"descuento_bs"`
	Items        []SaleLine    `json:"items"`
	Partes       []PaymentPart `json:"partes"`
}

// SaleCompletion is returned by the atomic completion operation and reused for
// idempotent retries (RT-03). Comandas contains one entry per preparation area
// represented in the sale (AC1); SinArea lists products that have no area
// assigned and therefore generate no ticket.
type SaleCompletion struct {
	Venta         Sale      `json:"venta"`
	StockNegativo bool      `json:"stock_negativo"`
	Comandas      []Comanda `json:"comandas"`
	SinArea       []string  `json:"sin_area"`
}

// AreaPreparacion is a configurable preparation area (RF-26/O-05).
type AreaPreparacion struct {
	ID             int64  `json:"id"`
	Nombre         string `json:"nombre"`
	OrdenImpresion int    `json:"orden_impresion"`
	Activo         bool   `json:"activo"`
}

// CreateAreaInput creates a preparation area without code changes (O-05).
type CreateAreaInput struct {
	UsuarioID      int64  `json:"usuario_id"`
	Nombre         string `json:"nombre"`
	OrdenImpresion int    `json:"orden_impresion"`
}

// UpdateAreaInput edits or deactivates an existing area. Nil fields are kept.
type UpdateAreaInput struct {
	UsuarioID      int64   `json:"usuario_id"`
	Nombre         *string `json:"nombre"`
	OrdenImpresion *int    `json:"orden_impresion"`
	Activo         *bool   `json:"activo"`
}

// AssignProductAreaInput sets (or clears, with null) the area of a product.
type AssignProductAreaInput struct {
	UsuarioID         int64  `json:"usuario_id"`
	AreaPreparacionID *int64 `json:"area_preparacion_id"`
}

// ProductoArea is the product/area assignment returned by the API.
type ProductoArea struct {
	ProductoID        int64  `json:"producto_id"`
	Codigo            string `json:"codigo"`
	Nombre            string `json:"nombre"`
	AreaPreparacionID *int64 `json:"area_preparacion_id"`
}

// ComandaItem is a snapshotted ticket line (name at sale time).
type ComandaItem struct {
	ID             int64  `json:"id"`
	ComandaID      int64  `json:"comanda_id"`
	VentaDetalleID int64  `json:"venta_detalle_id"`
	ProductoID     int64  `json:"producto_id"`
	Cantidad       int64  `json:"cantidad"`
	Nombre         string `json:"nombre"`
}

// Comanda is one printable ticket for one preparation area of a sale.
type Comanda struct {
	ID                int64         `json:"id"`
	VentaID           int64         `json:"venta_id"`
	AreaPreparacionID int64         `json:"area_preparacion_id"`
	AreaNombre        string        `json:"area_nombre"`
	OrdenImpresion    int           `json:"orden_impresion"`
	UsuarioID         int64         `json:"usuario_id"`
	Estado            string        `json:"estado"`
	Impresa           bool          `json:"impresa"`
	Copia             bool          `json:"copia"`
	ImpresaEn         *time.Time    `json:"impresa_en,omitempty"`
	CreadoEn          time.Time     `json:"creado_en"`
	Items             []ComandaItem `json:"items"`
}

// Impresion is the insert-only print audit row (RF-25/RNF-03).
type Impresion struct {
	ID            int64     `json:"id"`
	DocumentoTipo string    `json:"documento_tipo"`
	DocumentoID   int64     `json:"documento_id"`
	VentaID       int64     `json:"venta_id"`
	ComandaID     *int64    `json:"comanda_id,omitempty"`
	NumeroCopia   int       `json:"numero_copia"`
	Copia         bool      `json:"copia"`
	UsuarioID     int64     `json:"usuario_id"`
	CreadoEn      time.Time `json:"creado_en"`
}

// PrintDocument is a print-ready representation returned to the browser.
// Formato is "html" or "texto"; Contenido can be sent directly to
// window.print()/a print dialog without any hardware driver (AC4).
type PrintDocument struct {
	Formato   string    `json:"formato"`
	Contenido string    `json:"contenido"`
	Impresion Impresion `json:"impresion"`
}

// CajaSesion is an open/closed cash session.
type CajaSesion struct {
	ID                 int64      `json:"id"`
	Estado             string     `json:"estado"`
	FechaApertura      time.Time  `json:"fecha_apertura"`
	UsuarioAperturaID  int64      `json:"usuario_apertura_id"`
	MontoInicialBs     int64      `json:"monto_inicial_bs"`
	FechaCierre        *time.Time `json:"fecha_cierre,omitempty"`
	UsuarioCierreID    *int64     `json:"usuario_cierre_id,omitempty"`
	EfectivoEsperadoBs *int64     `json:"efectivo_esperado_bs,omitempty"`
	EfectivoContadoBs  *int64     `json:"efectivo_contado_bs,omitempty"`
	DiferenciaBs       *int64     `json:"diferencia_bs,omitempty"`
}

// OpenCajaInput opens the single allowed cash session (CJ-01/CJ-02).
type OpenCajaInput struct {
	UsuarioID      int64 `json:"usuario_id"`
	MontoInicialBs int64 `json:"monto_inicial_bs"`
}

// CloseCajaInput records counted cash and the difference (CJ-03).
type CloseCajaInput struct {
	UsuarioID         int64 `json:"usuario_id"`
	EfectivoContadoBs int64 `json:"efectivo_contado_bs"`
}
