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

// CajaMovimiento is one immutable cash movement of a session (RF-23/RT-04).
type CajaMovimiento struct {
	ID        int64     `json:"id"`
	SesionID  int64     `json:"sesion_id"`
	Tipo      string    `json:"tipo"`
	MontoBs   int64     `json:"monto_bs"`
	Motivo    *string   `json:"motivo,omitempty"`
	UsuarioID int64     `json:"usuario_id"`
	VentaID   *int64    `json:"venta_id,omitempty"`
	CreadoEn  time.Time `json:"creado_en"`
}

// CajaMovimientoInput registers an entrada or salida of cash. Tipo is optional
// on the dedicated entry/exit endpoints, where the server sets it.
type CajaMovimientoInput struct {
	UsuarioID int64  `json:"usuario_id"`
	Tipo      string `json:"tipo,omitempty"`
	MontoBs   int64  `json:"monto_bs"`
	Motivo    string `json:"motivo"`
}

// CajaVentaEnProceso is the CJ-04 warning detail: a draft sale that does not
// contribute to expected cash until it is completed.
type CajaVentaEnProceso struct {
	ID        int64     `json:"id"`
	UsuarioID int64     `json:"usuario_id"`
	TotalBs   int64     `json:"total_bs"`
	Fecha     time.Time `json:"fecha"`
}

// CajaCierreResult is the closed session plus the CJ-04 warning. The session
// fields stay at the top level for compatibility with the existing endpoint.
type CajaCierreResult struct {
	CajaSesion
	Aviso                  string               `json:"aviso,omitempty"`
	VentasEnProceso        int64                `json:"ventas_en_proceso"`
	VentasEnProcesoDetalle []CajaVentaEnProceso `json:"ventas_en_proceso_detalle"`
}

// Producto is the catalog row used by the inventory API (RF-01).
type Producto struct {
	ID                int64     `json:"id"`
	Codigo            string    `json:"codigo"`
	Nombre            string    `json:"nombre"`
	CategoriaID       *int64    `json:"categoria_id,omitempty"`
	AreaPreparacionID *int64    `json:"area_preparacion_id,omitempty"`
	Unidad            string    `json:"unidad"`
	StockMinimo       int64     `json:"stock_minimo"`
	PrecioVentaBs     int64     `json:"precio_venta_bs"`
	Activo            bool      `json:"activo"`
	CreadoEn          time.Time `json:"creado_en"`
	ActualizadoEn     time.Time `json:"actualizado_en"`
}

// CreateProductoInput registers a product. Stock is never set directly here:
// it only changes through inventory movements or sales (AC12).
type CreateProductoInput struct {
	UsuarioID     int64  `json:"usuario_id"`
	Codigo        string `json:"codigo"`
	Nombre        string `json:"nombre"`
	CategoriaID   *int64 `json:"categoria_id"`
	Unidad        string `json:"unidad"`
	StockMinimo   int64  `json:"stock_minimo"`
	PrecioVentaBs int64  `json:"precio_venta_bs"`
}

// UpdateProductoInput edits a product. Nil fields are kept.
type UpdateProductoInput struct {
	UsuarioID     int64   `json:"usuario_id"`
	Codigo        *string `json:"codigo"`
	Nombre        *string `json:"nombre"`
	CategoriaID   *int64  `json:"categoria_id"`
	Unidad        *string `json:"unidad"`
	StockMinimo   *int64  `json:"stock_minimo"`
	PrecioVentaBs *int64  `json:"precio_venta_bs"`
	Activo        *bool   `json:"activo"`
}

// Deposito is a physical storage location (RF-05).
type Deposito struct {
	ID            int64     `json:"id"`
	Nombre        string    `json:"nombre"`
	Ubicacion     *string   `json:"ubicacion,omitempty"`
	Activo        bool      `json:"activo"`
	CreadoEn      time.Time `json:"creado_en"`
	ActualizadoEn time.Time `json:"actualizado_en"`
}

// CreateDepositoInput registers a deposit.
type CreateDepositoInput struct {
	UsuarioID int64   `json:"usuario_id"`
	Nombre    string  `json:"nombre"`
	Ubicacion *string `json:"ubicacion"`
}

// UpdateDepositoInput edits or deactivates a deposit. Nil fields are kept.
type UpdateDepositoInput struct {
	UsuarioID int64   `json:"usuario_id"`
	Nombre    *string `json:"nombre"`
	Ubicacion *string `json:"ubicacion"`
	Activo    *bool   `json:"activo"`
}

// Proveedor is a supplier (RF-08).
type Proveedor struct {
	ID             int64     `json:"id"`
	Nombre         string    `json:"nombre"`
	ContactoNombre *string   `json:"contacto_nombre,omitempty"`
	Telefono       *string   `json:"telefono,omitempty"`
	Email          *string   `json:"email,omitempty"`
	Direccion      *string   `json:"direccion,omitempty"`
	Activo         bool      `json:"activo"`
	CreadoEn       time.Time `json:"creado_en"`
	ActualizadoEn  time.Time `json:"actualizado_en"`
}

// CreateProveedorInput registers a supplier.
type CreateProveedorInput struct {
	UsuarioID      int64   `json:"usuario_id"`
	Nombre         string  `json:"nombre"`
	ContactoNombre *string `json:"contacto_nombre"`
	Telefono       *string `json:"telefono"`
	Email          *string `json:"email"`
	Direccion      *string `json:"direccion"`
}

// UpdateProveedorInput edits or deactivates a supplier. Nil fields are kept.
type UpdateProveedorInput struct {
	UsuarioID      int64   `json:"usuario_id"`
	Nombre         *string `json:"nombre"`
	ContactoNombre *string `json:"contacto_nombre"`
	Telefono       *string `json:"telefono"`
	Email          *string `json:"email"`
	Direccion      *string `json:"direccion"`
	Activo         *bool   `json:"activo"`
}

// ProductoProveedor is the item-supplier association (RF-07).
type ProductoProveedor struct {
	ProductoID      int64     `json:"producto_id"`
	ProveedorID     int64     `json:"proveedor_id"`
	ProveedorNombre string    `json:"proveedor_nombre"`
	ContactoNombre  *string   `json:"contacto_nombre,omitempty"`
	Telefono        *string   `json:"telefono,omitempty"`
	Email           *string   `json:"email,omitempty"`
	Direccion       *string   `json:"direccion,omitempty"`
	ProveedorActivo bool      `json:"proveedor_activo"`
	EsPrincipal     bool      `json:"es_principal"`
	CreadoEn        time.Time `json:"creado_en"`
}

// AsociarProveedorInput adds or updates an item-supplier association.
type AsociarProveedorInput struct {
	UsuarioID   int64 `json:"usuario_id"`
	ProveedorID int64 `json:"proveedor_id"`
	EsPrincipal bool  `json:"es_principal"`
}

// StockDeposito is the quantity of one product in one deposit.
type StockDeposito struct {
	DepositoID     int64  `json:"deposito_id"`
	DepositoNombre string `json:"deposito_nombre"`
	Cantidad       int64  `json:"cantidad"`
}

// StockProducto is the stock of a product per deposit plus its total.
type StockProducto struct {
	ProductoID  int64           `json:"producto_id"`
	Codigo      string          `json:"codigo"`
	Nombre      string          `json:"nombre"`
	StockMinimo int64           `json:"stock_minimo"`
	StockTotal  int64           `json:"stock_total"`
	Depositos   []StockDeposito `json:"depositos"`
}

// InventarioMovimiento is one immutable inventory history row
// (RF-02/RF-03/RF-06).
type InventarioMovimiento struct {
	ID                      int64     `json:"id"`
	ProductoID              int64     `json:"producto_id"`
	ProductoCodigo          string    `json:"producto_codigo,omitempty"`
	ProductoNombre          string    `json:"producto_nombre,omitempty"`
	DepositoID              int64     `json:"deposito_id"`
	DepositoNombre          string    `json:"deposito_nombre,omitempty"`
	Tipo                    string    `json:"tipo"`
	Cantidad                int64     `json:"cantidad"`
	StockNegativoConfirmado bool      `json:"stock_negativo_confirmado"`
	GrupoID                 string    `json:"grupo_id,omitempty"`
	VentaID                 *int64    `json:"venta_id,omitempty"`
	UsuarioID               int64     `json:"usuario_id"`
	Motivo                  *string   `json:"motivo,omitempty"`
	CreadoEn                time.Time `json:"creado_en"`
}

// InventarioMovimientoInput is a manual entrada, salida or ajuste. Cantidad is
// always captured positive; the server applies the sign (RT-02).
type InventarioMovimientoInput struct {
	UsuarioID              int64  `json:"usuario_id"`
	ProductoID             int64  `json:"producto_id"`
	DepositoID             int64  `json:"deposito_id"`
	Tipo                   string `json:"tipo"`
	Cantidad               int64  `json:"cantidad"`
	Sentido                string `json:"sentido,omitempty"`
	Motivo                 string `json:"motivo,omitempty"`
	ConfirmarStockNegativo bool   `json:"confirmar_stock_negativo"`
}

// MovimientoResultado reports the inserted movement and the resulting stock of
// the affected product/deposit (RF-02/RF-03).
type MovimientoResultado struct {
	Movimiento      InventarioMovimiento `json:"movimiento"`
	StockResultante int64                `json:"stock_resultante"`
}

// TrasladoInput moves stock between two deposits (RF-06/ST-02).
type TrasladoInput struct {
	UsuarioID              int64  `json:"usuario_id"`
	ProductoID             int64  `json:"producto_id"`
	DepositoOrigenID       int64  `json:"deposito_origen_id"`
	DepositoDestinoID      int64  `json:"deposito_destino_id"`
	Cantidad               int64  `json:"cantidad"`
	Motivo                 string `json:"motivo,omitempty"`
	ConfirmarStockNegativo bool   `json:"confirmar_stock_negativo"`
}

// TrasladoResultado reports both persisted halves of one atomic transfer.
type TrasladoResultado struct {
	GrupoID      string               `json:"grupo_id"`
	Origen       InventarioMovimiento `json:"origen"`
	Destino      InventarioMovimiento `json:"destino"`
	StockOrigen  int64                `json:"stock_origen"`
	StockDestino int64                `json:"stock_destino"`
}

// BajoStockProducto is one active product at or below its minimum.
type BajoStockProducto struct {
	ProductoID  int64           `json:"producto_id"`
	Codigo      string          `json:"codigo"`
	Nombre      string          `json:"nombre"`
	StockMinimo int64           `json:"stock_minimo"`
	StockTotal  int64           `json:"stock_total"`
	Depositos   []StockDeposito `json:"depositos"`
}

// BajoStockGrupo groups low-stock products by principal supplier (RF-04).
type BajoStockGrupo struct {
	ProveedorID     *int64              `json:"proveedor_id"`
	ProveedorNombre string              `json:"proveedor_nombre"`
	ProveedorActivo *bool               `json:"proveedor_activo"`
	Productos       []BajoStockProducto `json:"productos"`
}

// BajoStockReporte is the grouped low-stock report.
type BajoStockReporte struct {
	Grupos []BajoStockGrupo `json:"grupos"`
}

// CostoCompraInput registers one row of the insert-only purchase-cost history
// (RF-27). Fecha uses the ISO date format YYYY-MM-DD.
type CostoCompraInput struct {
	UsuarioID   int64  `json:"usuario_id"`
	ProveedorID *int64 `json:"proveedor_id"`
	CostoBs     int64  `json:"costo_bs"`
	Fecha       string `json:"fecha"`
}

// CostoCompra is one immutable purchase-cost history row. CostoBs is the cost
// per minimum unit of the product, not the total paid for a batch (RT-01).
type CostoCompra struct {
	ID              int64     `json:"id"`
	ProductoID      int64     `json:"producto_id"`
	ProveedorID     *int64    `json:"proveedor_id,omitempty"`
	ProveedorNombre string    `json:"proveedor_nombre,omitempty"`
	CostoBs         int64     `json:"costo_bs"`
	Fecha           string    `json:"fecha"`
	UsuarioID       int64     `json:"usuario_id"`
	CreadoEn        time.Time `json:"creado_en"`
}

// RecetaItemInput is one ingredient line of a recipe replacement.
type RecetaItemInput struct {
	IngredienteID int64 `json:"ingrediente_id"`
	Cantidad      int64 `json:"cantidad"`
}

// UpdateRecetaInput replaces the complete recipe of a product. An empty Items
// list removes the recipe; Rendimiento nil keeps the existing value (or 1).
type UpdateRecetaInput struct {
	UsuarioID   int64             `json:"usuario_id"`
	Rendimiento *int64            `json:"rendimiento"`
	Items       []RecetaItemInput `json:"items"`
}

// RecetaItem is one ingredient of the current recipe, enriched with catalog
// data so the API response is directly usable by the UI.
type RecetaItem struct {
	IngredienteID int64  `json:"ingrediente_id"`
	Codigo        string `json:"codigo"`
	Nombre        string `json:"nombre"`
	Unidad        string `json:"unidad"`
	Cantidad      int64  `json:"cantidad"`
}

// Receta is the current recipe header plus its ingredients. Rendimiento is
// batch metadata in v1: the cost formula is the sum of ingredient costs.
type Receta struct {
	ProductoID    int64        `json:"producto_id"`
	Rendimiento   int64        `json:"rendimiento"`
	Items         []RecetaItem `json:"items"`
	CreadoEn      *time.Time   `json:"creado_en,omitempty"`
	ActualizadoEn *time.Time   `json:"actualizado_en,omitempty"`
}

// IngredienteSinCosto identifies a product that prevents a complete cost
// calculation because it has neither purchase cost nor recipe (AC6).
type IngredienteSinCosto struct {
	ProductoID int64  `json:"producto_id"`
	Codigo     string `json:"codigo"`
	Nombre     string `json:"nombre"`
}

// CostoMargen is the computed cost/margin of one product (RF-28/RF-29).
// CostoBs and MargenBs are nil when the cost is incomplete; nil is never
// replaced by zero because that would silently understate costs.
type CostoMargen struct {
	ProductoID           int64                 `json:"producto_id"`
	PrecioVentaBs        int64                 `json:"precio_venta_bs"`
	CostoBs              *int64                `json:"costo_bs"`
	MargenBs             *int64                `json:"margen_bs"`
	Origen               string                `json:"origen"`
	CostoCompleto        bool                  `json:"costo_completo"`
	IngredientesSinCosto []IngredienteSinCosto `json:"ingredientes_sin_costo"`
}

// UsuarioSesion is the authenticated identity returned by login and /me. It
// never contains the password hash or session token (RNF-02).
type UsuarioSesion struct {
	ID             int64  `json:"id"`
	NombreCompleto string `json:"nombre_completo"`
	Login          string `json:"login"`
	Rol            string `json:"rol"`
}

// Usuario is an administration listing row without password_hash.
type Usuario struct {
	ID             int64     `json:"id"`
	NombreCompleto string    `json:"nombre_completo"`
	Login          string    `json:"login"`
	Rol            string    `json:"rol"`
	Activo         bool      `json:"activo"`
	CreadoEn       time.Time `json:"creado_en"`
	ActualizadoEn  time.Time `json:"actualizado_en"`
}

// CreateUsuarioInput registers a user with an immutable role (RF-09/RF-10).
// UsuarioID is the authenticated actor and is always overwritten by the HTTP
// transport from the server-side session.
type CreateUsuarioInput struct {
	UsuarioID      int64  `json:"usuario_id"`
	NombreCompleto string `json:"nombre_completo"`
	Login          string `json:"login"`
	Password       string `json:"password"`
	Rol            string `json:"rol"`
}

// UpdateUsuarioInput edits the name, activates/deactivates or resets the
// password. Rol is accepted only to produce an explicit immutability error.
type UpdateUsuarioInput struct {
	UsuarioID      int64   `json:"usuario_id"`
	NombreCompleto *string `json:"nombre_completo"`
	Activo         *bool   `json:"activo"`
	Password       *string `json:"password"`
	Rol            *string `json:"rol"`
}

// LoginResult carries the public identity plus the raw token and expiry that
// the HTTP layer turns into the session cookie. The raw token is never logged
// or persisted; only its SHA-256 hash is stored.
type LoginResult struct {
	Usuario  UsuarioSesion
	Token    string
	ExpiraEn time.Time
}

// BootstrapConfig is the environment-provided first owner. It is used only
// when the usuarios table is empty (AC8).
type BootstrapConfig struct {
	Login    string
	Password string
	Nombre   string
}
