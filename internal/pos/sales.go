package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"bengalpos/internal/domain"
)

var errProductoNoDisponible = appError("producto_no_disponible", "El producto no está disponible", 422)

// CreateSale creates an "en_proceso" sale, sealing the catalog price of every
// line at creation time (PG-04) and validating integer amounts/discounts.
func (s *Service) CreateSale(ctx context.Context, in CreateSaleInput) (*Sale, error) {
	if in.UsuarioID <= 0 {
		return nil, ErrNoAutorizado
	}
	if len(in.Items) == 0 {
		return nil, ErrVentaSinItems
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create sale: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := userRole(ctx, tx, in.UsuarioID); err != nil {
		return nil, err
	}

	seen := make(map[int64]bool, len(in.Items))
	lines := make([]domain.SaleLine, 0, len(in.Items))
	prices := make([]int64, 0, len(in.Items))
	productos := make([]struct {
		ID     int64
		Codigo string
		Nombre string
	}, 0, len(in.Items))

	for _, item := range in.Items {
		if item.Cantidad <= 0 {
			return nil, ErrCantidad
		}
		if item.ProductoID <= 0 {
			return nil, appError("producto_no_encontrado", "Producto no encontrado", 422)
		}
		if seen[item.ProductoID] {
			return nil, appError("producto_duplicado", "El producto está repetido en la venta", 422)
		}
		seen[item.ProductoID] = true

		var codigo, nombre string
		var precio int64
		var activo bool
		err := tx.QueryRow(ctx,
			`SELECT codigo, nombre, precio_venta_bs, activo FROM productos WHERE id = $1`,
			item.ProductoID,
		).Scan(&codigo, &nombre, &precio, &activo)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appError("producto_no_encontrado", fmt.Sprintf("Producto no encontrado: %d", item.ProductoID), 422)
		}
		if err != nil {
			return nil, fmt.Errorf("load product: %w", err)
		}
		if !activo {
			return nil, errProductoNoDisponible
		}

		line := domain.SaleLine{Cantidad: item.Cantidad, PrecioUnitarioBs: precio}
		if _, err := line.Subtotal(); err != nil {
			return nil, mapDomainError(err)
		}
		lines = append(lines, line)
		prices = append(prices, precio)
		productos = append(productos, struct {
			ID     int64
			Codigo string
			Nombre string
		}{ID: item.ProductoID, Codigo: codigo, Nombre: nombre})
	}

	subtotal, err := domain.SaleTotal(lines)
	if err != nil {
		return nil, mapDomainError(err)
	}
	if err := domain.ValidateDiscount(in.DescuentoBs, subtotal); err != nil {
		return nil, mapDomainError(err)
	}
	if in.DescuentoBs > 0 {
		if in.DescuentoUsuarioID == nil || *in.DescuentoUsuarioID <= 0 {
			return nil, ErrNoAutorizado
		}
		rol, err := userRole(ctx, tx, *in.DescuentoUsuarioID)
		if err != nil || !canAuthorize(rol) {
			return nil, ErrNoAutorizado
		}
	}
	total := subtotal - in.DescuentoBs

	var saleID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO ventas (estado, usuario_id, total_bs, descuento_bs, descuento_usuario_id)
		 VALUES ('en_proceso', $1, $2, $3, $4) RETURNING id`,
		in.UsuarioID, total, in.DescuentoBs, in.DescuentoUsuarioID,
	).Scan(&saleID)
	if err != nil {
		return nil, fmt.Errorf("insert sale: %w", err)
	}
	if in.DescuentoBs > 0 {
		if _, err := tx.Exec(ctx,
			`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
			 VALUES ($1, 'descuento_autorizado', 'venta', $2,
			         jsonb_build_object('descuento_bs', $3::bigint))`,
			*in.DescuentoUsuarioID, saleID, in.DescuentoBs,
		); err != nil {
			return nil, fmt.Errorf("audit authorized discount: %w", err)
		}
	}

	for i, item := range in.Items {
		lineSubtotal := item.Cantidad * prices[i]
		if _, err := tx.Exec(ctx,
			`INSERT INTO venta_detalles
			 (venta_id, producto_id, cantidad, precio_unitario_bs, descuento_bs, subtotal_bs)
			 VALUES ($1, $2, $3, $4, 0, $5)`,
			saleID, item.ProductoID, item.Cantidad, prices[i], lineSubtotal,
		); err != nil {
			return nil, fmt.Errorf("insert sale detail: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create sale: %w", err)
	}
	return s.GetSale(ctx, saleID)
}

// GetSale loads the sale aggregate (items and payment parts).
func (s *Service) GetSale(ctx context.Context, saleID int64) (*Sale, error) {
	return getSale(ctx, s.pool, saleID)
}

func getSale(ctx context.Context, q dbQuerier, saleID int64) (*Sale, error) {
	var sale Sale
	err := q.QueryRow(ctx,
		`SELECT id, estado, usuario_id, caja_sesion_id, fecha, total_bs, descuento_bs
		 FROM ventas WHERE id = $1`, saleID,
	).Scan(&sale.ID, &sale.Estado, &sale.UsuarioID, &sale.CajaSesionID, &sale.Fecha, &sale.TotalBs, &sale.DescuentoBs)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrVentaNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load sale: %w", err)
	}

	sale.Items = []SaleLine{}
	rows, err := q.Query(ctx,
		`SELECT d.id, d.producto_id, p.codigo, p.nombre, d.cantidad,
		        d.precio_unitario_bs, d.descuento_bs, d.subtotal_bs
		 FROM venta_detalles d
		 JOIN productos p ON p.id = d.producto_id
		 WHERE d.venta_id = $1
		 ORDER BY d.id`, saleID)
	if err != nil {
		return nil, fmt.Errorf("load sale details: %w", err)
	}
	for rows.Next() {
		var line SaleLine
		if err := rows.Scan(&line.ID, &line.ProductoID, &line.ProductoCodigo, &line.ProductoNombre,
			&line.Cantidad, &line.PrecioUnitarioBs, &line.DescuentoBs, &line.SubtotalBs); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan sale detail: %w", err)
		}
		sale.Items = append(sale.Items, line)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate sale details: %w", err)
	}
	rows.Close()

	sale.Partes = []PaymentPart{}
	rows, err = q.Query(ctx,
		`SELECT pp.id, pp.venta_id, m.codigo, m.nombre, pp.monto_bs, pp.estado,
		        pp.confirmado_por, pp.confirmado_en, pp.referencia, pp.creado_en
		 FROM partes_pago pp
		 JOIN metodos_pago m ON m.id = pp.metodo_id
		 WHERE pp.venta_id = $1
		 ORDER BY pp.id`, saleID)
	if err != nil {
		return nil, fmt.Errorf("load payment parts: %w", err)
	}
	for rows.Next() {
		var part PaymentPart
		if err := rows.Scan(&part.ID, &part.VentaID, &part.MetodoCodigo, &part.MetodoNombre,
			&part.MontoBs, &part.Estado, &part.ConfirmadoPor, &part.ConfirmadoEn,
			&part.Referencia, &part.CreadoEn); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan payment part: %w", err)
		}
		sale.Partes = append(sale.Partes, part)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate payment parts: %w", err)
	}
	rows.Close()
	return &sale, nil
}

type metodoPago struct {
	ID                  int64
	Codigo              string
	Nombre              string
	RequiereCajaAbierta bool
	Activo              bool
}

func loadMetodo(ctx context.Context, q dbQuerier, codigo string) (*metodoPago, error) {
	var m metodoPago
	err := q.QueryRow(ctx,
		`SELECT id, codigo, nombre, requiere_caja_abierta, activo
		 FROM metodos_pago WHERE codigo = $1`, codigo,
	).Scan(&m.ID, &m.Codigo, &m.Nombre, &m.RequiereCajaAbierta, &m.Activo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMetodoPago
	}
	if err != nil {
		return nil, fmt.Errorf("load payment method: %w", err)
	}
	if !m.Activo {
		return nil, ErrMetodoPago
	}
	return &m, nil
}

func openCajaID(ctx context.Context, q dbQuerier, forUpdate bool) (int64, error) {
	query := `SELECT id FROM caja_sesiones WHERE estado = 'abierta'`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var id int64
	err := q.QueryRow(ctx, query).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrCajaRequerida
	}
	if err != nil {
		return 0, fmt.Errorf("load open cash session: %w", err)
	}
	return id, nil
}

func getPart(ctx context.Context, q dbQuerier, ventaID, partID int64) (*PaymentPart, error) {
	var part PaymentPart
	err := q.QueryRow(ctx,
		`SELECT pp.id, pp.venta_id, m.codigo, m.nombre, pp.monto_bs, pp.estado,
		        pp.confirmado_por, pp.confirmado_en, pp.referencia, pp.creado_en
		 FROM partes_pago pp
		 JOIN metodos_pago m ON m.id = pp.metodo_id
		 WHERE pp.venta_id = $1 AND pp.id = $2`, ventaID, partID,
	).Scan(&part.ID, &part.VentaID, &part.MetodoCodigo, &part.MetodoNombre, &part.MontoBs,
		&part.Estado, &part.ConfirmadoPor, &part.ConfirmadoEn, &part.Referencia, &part.CreadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrParteNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load payment part: %w", err)
	}
	return &part, nil
}

// AddPaymentPart registers a pending part. Cash requires an open cash session
// (PG-03) and validates received cash/change (PG-01); the part is never
// confirmed automatically (PG-02).
func (s *Service) AddPaymentPart(ctx context.Context, ventaID int64, in AddPaymentPartInput) (*PaymentPart, error) {
	if in.MontoBs <= 0 {
		return nil, ErrMonto
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin add payment part: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := userRole(ctx, tx, in.UsuarioID); err != nil {
		return nil, err
	}

	var estado string
	err = tx.QueryRow(ctx, `SELECT estado FROM ventas WHERE id = $1 FOR UPDATE`, ventaID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrVentaNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load sale state: %w", err)
	}
	if estado == "anulada" {
		return nil, ErrVentaAnulada
	}
	if estado != "en_proceso" {
		return nil, ErrVentaCerrada
	}

	metodo, err := loadMetodo(ctx, tx, in.MetodoCodigo)
	if err != nil {
		return nil, err
	}
	if metodo.RequiereCajaAbierta {
		if _, err := openCajaID(ctx, tx, false); err != nil {
			return nil, err
		}
	}

	recibido := in.MontoBs
	if metodo.Codigo == "efectivo" {
		if in.RecibidoBs != nil {
			recibido = *in.RecibidoBs
		}
	}
	var cambio int64
	if metodo.Codigo == "efectivo" {
		cambio, err = domain.CashChange(in.MontoBs, recibido)
		if err != nil {
			return nil, mapDomainError(err)
		}
	}

	var partID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO partes_pago (venta_id, metodo_id, monto_bs, estado, referencia)
		 VALUES ($1, $2, $3, 'pendiente', $4) RETURNING id`,
		ventaID, metodo.ID, in.MontoBs, in.Referencia,
	).Scan(&partID)
	if err != nil {
		return nil, fmt.Errorf("insert payment part: %w", err)
	}

	part, err := getPart(ctx, tx, ventaID, partID)
	if err != nil {
		return nil, err
	}
	part.CambioBs = cambio
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit add payment part: %w", err)
	}
	return part, nil
}

// ConfirmPaymentPart applies the manual confirmation, recording user and time.
func (s *Service) ConfirmPaymentPart(ctx context.Context, ventaID, partID, usuarioID int64) (*PaymentPart, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin confirm part: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := userRole(ctx, tx, usuarioID); err != nil {
		return nil, err
	}

	var estado string
	err = tx.QueryRow(ctx, `SELECT estado FROM ventas WHERE id = $1 FOR UPDATE`, ventaID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrVentaNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load sale state: %w", err)
	}
	if estado == "anulada" {
		return nil, ErrVentaAnulada
	}
	if estado != "en_proceso" {
		return nil, ErrVentaCerrada
	}

	var partEstado string
	var metodoID int64
	err = tx.QueryRow(ctx,
		`SELECT estado, metodo_id FROM partes_pago WHERE id = $1 AND venta_id = $2 FOR UPDATE`,
		partID, ventaID,
	).Scan(&partEstado, &metodoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrParteNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load payment part state: %w", err)
	}
	if partEstado == "confirmado" {
		part, err := getPart(ctx, tx, ventaID, partID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit confirm part: %w", err)
		}
		return part, nil
	}

	var requiereCaja bool
	if err := tx.QueryRow(ctx,
		`SELECT requiere_caja_abierta FROM metodos_pago WHERE id = $1`,
		metodoID,
	).Scan(&requiereCaja); err != nil {
		return nil, fmt.Errorf("load payment method: %w", err)
	}
	if requiereCaja {
		if _, err := openCajaID(ctx, tx, false); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE partes_pago
		 SET estado = 'confirmado', confirmado_por = $3, confirmado_en = now()
		 WHERE id = $1 AND venta_id = $2`, partID, ventaID, usuarioID,
	); err != nil {
		return nil, fmt.Errorf("confirm payment part: %w", err)
	}

	part, err := getPart(ctx, tx, ventaID, partID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit confirm part: %w", err)
	}
	return part, nil
}

// CompleteSale confirms the sale in one transaction: validates the confirmed
// payment sum, locks/deducts stock, records inventory and cash movements,
// seals the sale and writes the audit entry (RT-03, RNF-04).
func (s *Service) CompleteSale(ctx context.Context, ventaID int64, in CompleteSaleInput) (*SaleCompletion, error) {
	const endpoint = "POST /api/ventas/{id}/completar"

	var idemKey pgtype.UUID
	if in.IdempotencyKey != "" {
		if err := idemKey.Scan(in.IdempotencyKey); err != nil {
			return nil, appError("idempotency_key_invalida", "La clave de idempotencia debe ser un UUID", 422)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin complete sale: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := userRole(ctx, tx, in.UsuarioID); err != nil {
		return nil, err
	}

	if in.IdempotencyKey != "" {
		var payload []byte
		err := tx.QueryRow(ctx,
			`SELECT respuesta FROM idempotencia_claves WHERE clave = $1 AND endpoint = $2`,
			idemKey, endpoint,
		).Scan(&payload)
		if err == nil {
			var completion SaleCompletion
			if err := json.Unmarshal(payload, &completion); err != nil {
				return nil, fmt.Errorf("decode idempotent response: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, fmt.Errorf("commit idempotent response: %w", err)
			}
			return &completion, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("load idempotency key: %w", err)
		}
	}

	var sale Sale
	err = tx.QueryRow(ctx,
		`SELECT id, estado, usuario_id, caja_sesion_id, fecha, total_bs, descuento_bs
		 FROM ventas WHERE id = $1 FOR UPDATE`, ventaID,
	).Scan(&sale.ID, &sale.Estado, &sale.UsuarioID, &sale.CajaSesionID, &sale.Fecha, &sale.TotalBs, &sale.DescuentoBs)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrVentaNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load sale for completion: %w", err)
	}
	if sale.Estado == "anulada" {
		return nil, ErrVentaAnulada
	}
	if sale.Estado == "completada" {
		completedSale, err := getSale(ctx, tx, ventaID)
		if err != nil {
			return nil, err
		}
		comandas, err := loadComandas(ctx, tx, ventaID)
		if err != nil {
			return nil, err
		}
		completion := &SaleCompletion{Venta: *completedSale, Comandas: comandas, SinArea: []string{}}
		if in.IdempotencyKey != "" {
			payload, err := json.Marshal(completion)
			if err != nil {
				return nil, fmt.Errorf("encode idempotent response: %w", err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO idempotencia_claves (clave, usuario_id, endpoint, respuesta)
				 VALUES ($1, $2, $3, $4::jsonb)`,
				idemKey, sale.UsuarioID, endpoint, string(payload),
			); err != nil {
				return nil, fmt.Errorf("store idempotency key: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit completed sale: %w", err)
		}
		return completion, nil
	}

	type lineInfo struct {
		Cantidad int64
		Subtotal int64
	}
	lines := make(map[int64]lineInfo)
	rows, err := tx.Query(ctx,
		`SELECT producto_id, cantidad, subtotal_bs
		 FROM venta_detalles
		 WHERE venta_id = $1
		 ORDER BY producto_id`, ventaID)
	if err != nil {
		return nil, fmt.Errorf("load sale lines for completion: %w", err)
	}
	var detailSum int64
	for rows.Next() {
		var productoID int64
		var info lineInfo
		if err := rows.Scan(&productoID, &info.Cantidad, &info.Subtotal); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan sale line for completion: %w", err)
		}
		infoExisting := lines[productoID]
		infoExisting.Cantidad += info.Cantidad
		infoExisting.Subtotal += info.Subtotal
		lines[productoID] = infoExisting
		detailSum += info.Subtotal
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate sale lines for completion: %w", err)
	}
	rows.Close()
	if len(lines) == 0 {
		return nil, ErrVentaSinItems
	}
	if detailSum-sale.DescuentoBs != sale.TotalBs {
		return nil, ErrDatosVenta
	}

	comandaSource, err := loadComandaSource(ctx, tx, ventaID)
	if err != nil {
		return nil, err
	}
	comandaGroups, sinArea, err := domain.SplitComandas(comandaSource)
	if err != nil {
		return nil, mapDomainError(err)
	}

	type partInfo struct {
		ID           int64
		Codigo       string
		RequiereCaja bool
		Monto        int64
		Estado       string
	}
	parts := []partInfo{}
	rows, err = tx.Query(ctx,
		`SELECT pp.id, m.codigo, m.requiere_caja_abierta, pp.monto_bs, pp.estado
		 FROM partes_pago pp
		 JOIN metodos_pago m ON m.id = pp.metodo_id
		 WHERE pp.venta_id = $1
		 ORDER BY pp.id
		 FOR UPDATE OF pp`, ventaID)
	if err != nil {
		return nil, fmt.Errorf("load payment parts for completion: %w", err)
	}
	for rows.Next() {
		var part partInfo
		if err := rows.Scan(&part.ID, &part.Codigo, &part.RequiereCaja, &part.Monto, &part.Estado); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan payment part for completion: %w", err)
		}
		parts = append(parts, part)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate payment parts for completion: %w", err)
	}
	rows.Close()

	confirmed := make([]int64, 0, len(parts))
	cashParts := make([]partInfo, 0)
	for _, part := range parts {
		if part.Estado != "confirmado" {
			return nil, ErrSumaPagos
		}
		confirmed = append(confirmed, part.Monto)
		if part.RequiereCaja || part.Codigo == "efectivo" {
			cashParts = append(cashParts, part)
		}
	}
	if len(parts) == 0 {
		return nil, ErrSumaPagos
	}
	if err := domain.ValidateConfirmedPayments(sale.TotalBs, confirmed); err != nil {
		return nil, mapDomainError(err)
	}

	var cajaSesionID *int64
	if len(cashParts) > 0 {
		openID, err := openCajaID(ctx, tx, true)
		if err != nil {
			return nil, err
		}
		cajaSesionID = &openID
	}

	depositoID := int64(0)
	if in.DepositoID != nil {
		err := tx.QueryRow(ctx, `SELECT id FROM depositos WHERE id = $1 AND activo`, *in.DepositoID).Scan(&depositoID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appError("deposito_no_encontrado", "El depósito no existe o está inactivo", 422)
		}
		if err != nil {
			return nil, fmt.Errorf("load deposit: %w", err)
		}
	} else {
		err := tx.QueryRow(ctx, `SELECT id FROM depositos WHERE activo ORDER BY id LIMIT 1`).Scan(&depositoID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoDeposito
		}
		if err != nil {
			return nil, fmt.Errorf("load default deposit: %w", err)
		}
	}

	productIDs := make([]int64, 0, len(lines))
	for productoID := range lines {
		productIDs = append(productIDs, productoID)
	}
	sort.Slice(productIDs, func(i, j int) bool { return productIDs[i] < productIDs[j] })

	stockNegativo := false
	type stockChange struct {
		productoID int64
		cantidad   int64
		negativo   bool
	}
	changes := make([]stockChange, 0, len(productIDs))
	for _, productoID := range productIDs {
		info := lines[productoID]
		var stockActual int64
		rowExists := true
		err := tx.QueryRow(ctx,
			`SELECT cantidad FROM stock_depositos
			 WHERE producto_id = $1 AND deposito_id = $2 FOR UPDATE`,
			productoID, depositoID,
		).Scan(&stockActual)
		if errors.Is(err, pgx.ErrNoRows) {
			rowExists = false
			stockActual = 0
		} else if err != nil {
			return nil, fmt.Errorf("lock stock: %w", err)
		}

		negativo := stockActual < info.Cantidad
		if negativo {
			if !in.ConfirmarStockNegativo {
				return nil, stockError(stockActual - info.Cantidad)
			}
			stockNegativo = true
		}

		if rowExists {
			if _, err := tx.Exec(ctx,
				`UPDATE stock_depositos
				 SET cantidad = cantidad - $3, actualizado_en = now()
				 WHERE producto_id = $1 AND deposito_id = $2`,
				productoID, depositoID, info.Cantidad,
			); err != nil {
				return nil, fmt.Errorf("update stock: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx,
				`INSERT INTO stock_depositos (producto_id, deposito_id, cantidad)
				 VALUES ($1, $2, $3)`,
				productoID, depositoID, -info.Cantidad,
			); err != nil {
				return nil, fmt.Errorf("insert stock: %w", err)
			}
		}

		changes = append(changes, stockChange{
			productoID: productoID,
			cantidad:   info.Cantidad,
			negativo:   negativo,
		})
	}

	for _, change := range changes {
		if _, err := tx.Exec(ctx,
			`INSERT INTO movimientos_inventario
			 (producto_id, deposito_id, tipo, cantidad, stock_negativo_confirmado, venta_id, usuario_id)
			 VALUES ($1, $2, 'venta', $3, $4, $5, $6)`,
			change.productoID, depositoID, -change.cantidad,
			change.negativo, ventaID, sale.UsuarioID,
		); err != nil {
			return nil, fmt.Errorf("insert inventory movement: %w", err)
		}
	}

	// One comanda per area is written in the same transaction that completes
	// the sale (AC1). Printing is a separate auditable step, so a print failure
	// never rolls the sale back (RT-03).
	comandas, err := insertComandas(ctx, tx, ventaID, sale.UsuarioID, comandaGroups)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE ventas
		 SET estado = 'completada', caja_sesion_id = $2, actualizado_en = now()
		 WHERE id = $1`, ventaID, cajaSesionID,
	); err != nil {
		return nil, fmt.Errorf("complete sale: %w", err)
	}

	var cashTotal int64
	if cajaSesionID != nil {
		for _, part := range cashParts {
			cashTotal += part.Monto
			if _, err := tx.Exec(ctx,
				`INSERT INTO caja_movimientos (sesion_id, tipo, monto_bs, usuario_id, venta_id)
				 VALUES ($1, 'venta_efectivo', $2, $3, $4)`,
				*cajaSesionID, part.Monto, sale.UsuarioID, ventaID,
			); err != nil {
				return nil, fmt.Errorf("insert cash movement: %w", err)
			}
		}
	}

	auditData, err := json.Marshal(map[string]any{
		"venta_id":       ventaID,
		"total_bs":       sale.TotalBs,
		"efectivo_bs":    cashTotal,
		"partes":         len(parts),
		"stock_negativo": stockNegativo,
	})
	if err != nil {
		return nil, fmt.Errorf("encode audit data: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'venta_completada', 'venta', $2, $3::jsonb)`,
		in.UsuarioID, ventaID, string(auditData),
	); err != nil {
		return nil, fmt.Errorf("insert audit entry: %w", err)
	}

	completedSale, err := getSale(ctx, tx, ventaID)
	if err != nil {
		return nil, err
	}
	completion := &SaleCompletion{
		Venta:         *completedSale,
		StockNegativo: stockNegativo,
		Comandas:      comandas,
		SinArea:       comandaItemNames(sinArea),
	}

	if in.IdempotencyKey != "" {
		payload, err := json.Marshal(completion)
		if err != nil {
			return nil, fmt.Errorf("encode idempotent response: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO idempotencia_claves (clave, usuario_id, endpoint, respuesta)
			 VALUES ($1, $2, $3, $4::jsonb)`,
			idemKey, sale.UsuarioID, endpoint, string(payload),
		); err != nil {
			return nil, fmt.Errorf("store idempotency key: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit complete sale: %w", err)
	}
	return completion, nil
}
