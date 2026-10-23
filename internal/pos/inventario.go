package pos

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// normalizeMovimiento validates a manual movement and applies the sign on the
// server side (RT-02). Cantidad is always captured positive by the client.
func normalizeMovimiento(tipo string, cantidad int64, sentido, motivo string) (string, int64, *string, error) {
	tipo = strings.ToLower(strings.TrimSpace(tipo))
	if tipo != "entrada" && tipo != "salida" && tipo != "ajuste" {
		return "", 0, nil, ErrTipoMovimientoInventario
	}
	if cantidad <= 0 {
		return "", 0, nil, ErrCantidad
	}

	motivo = strings.TrimSpace(motivo)
	motivoPtr := (*string)(nil)
	if motivo != "" {
		motivoPtr = &motivo
	}

	signed := cantidad
	switch tipo {
	case "salida":
		signed = -cantidad
	case "ajuste":
		sentido = strings.ToLower(strings.TrimSpace(sentido))
		if sentido != "sube" && sentido != "baja" {
			return "", 0, nil, ErrSentidoAjuste
		}
		if motivoPtr == nil {
			return "", 0, nil, ErrMotivoAjusteRequerido
		}
		if sentido == "baja" {
			signed = -cantidad
		}
	}
	return tipo, signed, motivoPtr, nil
}

func ensureProductoExists(ctx context.Context, q dbQuerier, productoID int64) error {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM productos WHERE id = $1)`, productoID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check product: %w", err)
	}
	if !exists {
		return ErrProductoNoExiste
	}
	return nil
}

func ensureDepositoActivo(ctx context.Context, q dbQuerier, depositoID int64) error {
	deposito, err := loadDeposito(ctx, q, depositoID)
	if err != nil {
		return err
	}
	if !deposito.Activo {
		return ErrDepositoInactivo
	}
	return nil
}

// lockStock locks the product/deposit row, creating it at zero when it does not
// exist yet. The caller must already be inside a transaction.
func lockStock(ctx context.Context, tx pgx.Tx, productoID, depositoID int64) (int64, error) {
	var cantidad int64
	err := tx.QueryRow(ctx,
		`SELECT cantidad FROM stock_depositos
		 WHERE producto_id = $1 AND deposito_id = $2 FOR UPDATE`,
		productoID, depositoID,
	).Scan(&cantidad)
	if err == nil {
		return cantidad, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("lock stock: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO stock_depositos (producto_id, deposito_id, cantidad)
		 VALUES ($1, $2, 0)
		 ON CONFLICT (producto_id, deposito_id) DO NOTHING`,
		productoID, depositoID,
	); err != nil {
		return 0, fmt.Errorf("create stock row: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT cantidad FROM stock_depositos
		 WHERE producto_id = $1 AND deposito_id = $2 FOR UPDATE`,
		productoID, depositoID,
	).Scan(&cantidad); err != nil {
		return 0, fmt.Errorf("lock created stock row: %w", err)
	}
	return cantidad, nil
}

func setStock(ctx context.Context, tx pgx.Tx, productoID, depositoID, cantidad int64) error {
	if _, err := tx.Exec(ctx,
		`UPDATE stock_depositos
		 SET cantidad = $3, actualizado_en = now()
		 WHERE producto_id = $1 AND deposito_id = $2`,
		productoID, depositoID, cantidad,
	); err != nil {
		return fmt.Errorf("update stock: %w", err)
	}
	return nil
}

const movimientoSelect = `SELECT m.id, m.producto_id, p.codigo, p.nombre, m.deposito_id,
	d.nombre, m.tipo, m.cantidad, m.stock_negativo_confirmado,
	COALESCE(m.grupo_id::text, ''), m.venta_id, m.usuario_id, m.motivo, m.creado_en
	FROM movimientos_inventario m
	JOIN productos p ON p.id = m.producto_id
	JOIN depositos d ON d.id = m.deposito_id`

func scanMovimiento(row pgx.Row) (*InventarioMovimiento, error) {
	var m InventarioMovimiento
	if err := row.Scan(&m.ID, &m.ProductoID, &m.ProductoCodigo, &m.ProductoNombre,
		&m.DepositoID, &m.DepositoNombre, &m.Tipo, &m.Cantidad,
		&m.StockNegativoConfirmado, &m.GrupoID, &m.VentaID, &m.UsuarioID,
		&m.Motivo, &m.CreadoEn); err != nil {
		return nil, err
	}
	return &m, nil
}

func loadMovimiento(ctx context.Context, q dbQuerier, movimientoID int64) (*InventarioMovimiento, error) {
	movimiento, err := scanMovimiento(q.QueryRow(ctx,
		movimientoSelect+` WHERE m.id = $1`, movimientoID,
	))
	if err != nil {
		return nil, fmt.Errorf("load inventory movement: %w", err)
	}
	return movimiento, nil
}

func insertMovimiento(ctx context.Context, tx pgx.Tx, productoID, depositoID int64, tipo string,
	cantidad, usuarioID int64, motivo *string, negativo bool, grupoID *pgtype.UUID) (*InventarioMovimiento, error) {
	var grupo any
	if grupoID != nil {
		grupo = *grupoID
	}
	var movimientoID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO movimientos_inventario
		 (producto_id, deposito_id, tipo, cantidad, stock_negativo_confirmado,
		  grupo_id, usuario_id, motivo)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		productoID, depositoID, tipo, cantidad, negativo, grupo, usuarioID, motivo,
	).Scan(&movimientoID); err != nil {
		return nil, fmt.Errorf("insert inventory movement: %w", err)
	}
	return loadMovimiento(ctx, tx, movimientoID)
}

// AddMovimientoInventario registers a manual entrada, salida or ajuste and
// updates the stock projection in one transaction. A negative result requires
// explicit ST-01 confirmation; without it nothing is persisted.
func (s *Service) AddMovimientoInventario(ctx context.Context, in InventarioMovimientoInput) (*MovimientoResultado, error) {
	tipo, signed, motivo, err := normalizeMovimiento(in.Tipo, in.Cantidad, in.Sentido, in.Motivo)
	if err != nil {
		return nil, err
	}
	if in.ProductoID <= 0 {
		return nil, ErrProductoNoExiste
	}
	if in.DepositoID <= 0 {
		return nil, ErrDepositoNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin inventory movement: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canManageCatalogo(rol) {
		return nil, ErrNoAutorizado
	}
	if err := ensureProductoExists(ctx, tx, in.ProductoID); err != nil {
		return nil, err
	}
	if err := ensureDepositoActivo(ctx, tx, in.DepositoID); err != nil {
		return nil, err
	}

	actual, err := lockStock(ctx, tx, in.ProductoID, in.DepositoID)
	if err != nil {
		return nil, err
	}
	resultante := actual + signed
	negativo := signed < 0 && resultante < 0
	if negativo && !in.ConfirmarStockNegativo {
		return nil, stockError(resultante)
	}

	if err := setStock(ctx, tx, in.ProductoID, in.DepositoID, resultante); err != nil {
		return nil, err
	}
	movimiento, err := insertMovimiento(ctx, tx, in.ProductoID, in.DepositoID, tipo,
		signed, in.UsuarioID, motivo, negativo, nil)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'inventario_movimiento', 'movimiento_inventario', $2,
		         jsonb_build_object('producto_id', $3::bigint, 'deposito_id', $4::bigint,
		                            'tipo', $5::text, 'cantidad', $6::bigint,
		                            'stock_resultante', $7::bigint))`,
		in.UsuarioID, movimiento.ID, in.ProductoID, in.DepositoID,
		tipo, signed, resultante,
	); err != nil {
		return nil, fmt.Errorf("audit inventory movement: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit inventory movement: %w", err)
	}
	return &MovimientoResultado{Movimiento: *movimiento, StockResultante: resultante}, nil
}

func newUUID() (pgtype.UUID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return pgtype.UUID{}, fmt.Errorf("generate transfer group: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return pgtype.UUID{Bytes: b, Valid: true}, nil
}

// TransferirStock moves a positive quantity between active deposits in one
// transaction, inserting both halves with the same grupo_id (RF-06/ST-02).
func (s *Service) TransferirStock(ctx context.Context, in TrasladoInput) (*TrasladoResultado, error) {
	if in.Cantidad <= 0 {
		return nil, ErrCantidad
	}
	if in.ProductoID <= 0 {
		return nil, ErrProductoNoExiste
	}
	if in.DepositoOrigenID <= 0 || in.DepositoDestinoID <= 0 {
		return nil, ErrDepositoNoExiste
	}
	if in.DepositoOrigenID == in.DepositoDestinoID {
		return nil, ErrTrasladoMismoDeposito
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin stock transfer: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canManageCatalogo(rol) {
		return nil, ErrNoAutorizado
	}
	if err := ensureProductoExists(ctx, tx, in.ProductoID); err != nil {
		return nil, err
	}
	if err := ensureDepositoActivo(ctx, tx, in.DepositoOrigenID); err != nil {
		return nil, err
	}
	if err := ensureDepositoActivo(ctx, tx, in.DepositoDestinoID); err != nil {
		return nil, err
	}

	// Fixed lock order (deposit id) prevents deadlocks between concurrent
	// transfers in opposite directions (arquitectura §5).
	firstID, secondID := in.DepositoOrigenID, in.DepositoDestinoID
	if firstID > secondID {
		firstID, secondID = secondID, firstID
	}
	firstStock, err := lockStock(ctx, tx, in.ProductoID, firstID)
	if err != nil {
		return nil, err
	}
	secondStock, err := lockStock(ctx, tx, in.ProductoID, secondID)
	if err != nil {
		return nil, err
	}
	stockOrigen := firstStock
	stockDestino := secondStock
	if firstID != in.DepositoOrigenID {
		stockOrigen, stockDestino = secondStock, firstStock
	}

	nuevoOrigen := stockOrigen - in.Cantidad
	negativo := nuevoOrigen < 0
	if negativo && !in.ConfirmarStockNegativo {
		return nil, stockError(nuevoOrigen)
	}
	nuevoDestino := stockDestino + in.Cantidad

	if err := setStock(ctx, tx, in.ProductoID, in.DepositoOrigenID, nuevoOrigen); err != nil {
		return nil, err
	}
	if err := setStock(ctx, tx, in.ProductoID, in.DepositoDestinoID, nuevoDestino); err != nil {
		return nil, err
	}

	grupoID, err := newUUID()
	if err != nil {
		return nil, err
	}
	motivo := strings.TrimSpace(in.Motivo)
	var motivoPtr *string
	if motivo != "" {
		motivoPtr = &motivo
	}

	movimientoOrigen, err := insertMovimiento(ctx, tx, in.ProductoID, in.DepositoOrigenID,
		"traslado", -in.Cantidad, in.UsuarioID, motivoPtr, negativo, &grupoID)
	if err != nil {
		return nil, err
	}
	movimientoDestino, err := insertMovimiento(ctx, tx, in.ProductoID, in.DepositoDestinoID,
		"traslado", in.Cantidad, in.UsuarioID, motivoPtr, false, &grupoID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'inventario_traslado', 'movimiento_inventario', $2,
		         jsonb_build_object('producto_id', $3::bigint,
		                            'origen_id', $4::bigint, 'destino_id', $5::bigint,
		                            'cantidad', $6::bigint, 'grupo_id', $7::text))`,
		in.UsuarioID, movimientoOrigen.ID, in.ProductoID,
		in.DepositoOrigenID, in.DepositoDestinoID, in.Cantidad, grupoID.String(),
	); err != nil {
		return nil, fmt.Errorf("audit stock transfer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit stock transfer: %w", err)
	}
	return &TrasladoResultado{
		GrupoID:      grupoID.String(),
		Origen:       *movimientoOrigen,
		Destino:      *movimientoDestino,
		StockOrigen:  nuevoOrigen,
		StockDestino: nuevoDestino,
	}, nil
}

// GetStock returns the stock of each product per deposit plus its total. A
// product without a stock row is reported with total 0 (AC6).
func (s *Service) GetStock(ctx context.Context, productoID, depositoID *int64) ([]StockProducto, error) {
	productQuery := `SELECT id, codigo, nombre, stock_minimo FROM productos`
	productArgs := []any{}
	conditions := []string{}
	if productoID != nil {
		productArgs = append(productArgs, *productoID)
		conditions = append(conditions, fmt.Sprintf("id = $%d", len(productArgs)))
	}
	if len(conditions) > 0 {
		productQuery += ` WHERE ` + strings.Join(conditions, " AND ")
	}
	productQuery += ` ORDER BY codigo, id`

	rows, err := s.pool.Query(ctx, productQuery, productArgs...)
	if err != nil {
		return nil, fmt.Errorf("list stock products: %w", err)
	}
	productos := make([]StockProducto, 0)
	byID := make(map[int64]*StockProducto)
	for rows.Next() {
		var p StockProducto
		if err := rows.Scan(&p.ProductoID, &p.Codigo, &p.Nombre, &p.StockMinimo); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan stock product: %w", err)
		}
		p.Depositos = []StockDeposito{}
		productos = append(productos, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate stock products: %w", err)
	}
	rows.Close()
	for i := range productos {
		byID[productos[i].ProductoID] = &productos[i]
	}

	stockQuery := `SELECT sd.producto_id, sd.deposito_id, d.nombre, sd.cantidad
		FROM stock_depositos sd
		JOIN depositos d ON d.id = sd.deposito_id`
	stockArgs := []any{}
	stockConditions := []string{}
	if productoID != nil {
		stockArgs = append(stockArgs, *productoID)
		stockConditions = append(stockConditions, fmt.Sprintf("sd.producto_id = $%d", len(stockArgs)))
	}
	if depositoID != nil {
		stockArgs = append(stockArgs, *depositoID)
		stockConditions = append(stockConditions, fmt.Sprintf("sd.deposito_id = $%d", len(stockArgs)))
	}
	if len(stockConditions) > 0 {
		stockQuery += ` WHERE ` + strings.Join(stockConditions, " AND ")
	}
	stockQuery += ` ORDER BY sd.producto_id, d.nombre, sd.deposito_id`

	stockRows, err := s.pool.Query(ctx, stockQuery, stockArgs...)
	if err != nil {
		return nil, fmt.Errorf("list stock rows: %w", err)
	}
	defer stockRows.Close()
	for stockRows.Next() {
		var producto, deposito, cantidad int64
		var nombre string
		if err := stockRows.Scan(&producto, &deposito, &nombre, &cantidad); err != nil {
			return nil, fmt.Errorf("scan stock row: %w", err)
		}
		productoStock, ok := byID[producto]
		if !ok {
			continue
		}
		productoStock.Depositos = append(productoStock.Depositos, StockDeposito{
			DepositoID:     deposito,
			DepositoNombre: nombre,
			Cantidad:       cantidad,
		})
		productoStock.StockTotal += cantidad
	}
	if err := stockRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stock rows: %w", err)
	}
	return productos, nil
}

// ListMovimientos returns the immutable inventory history with optional filters
// (RF-02/RF-03/RF-06). There is no update/delete endpoint (AC12).
func (s *Service) ListMovimientos(ctx context.Context, productoID, depositoID *int64) ([]InventarioMovimiento, error) {
	query := movimientoSelect
	args := []any{}
	conditions := []string{}
	if productoID != nil {
		args = append(args, *productoID)
		conditions = append(conditions, fmt.Sprintf("m.producto_id = $%d", len(args)))
	}
	if depositoID != nil {
		args = append(args, *depositoID)
		conditions = append(conditions, fmt.Sprintf("m.deposito_id = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY m.creado_en, m.id`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list inventory movements: %w", err)
	}
	defer rows.Close()

	movimientos := make([]InventarioMovimiento, 0)
	for rows.Next() {
		movimiento, err := scanMovimiento(rows)
		if err != nil {
			return nil, fmt.Errorf("scan inventory movement: %w", err)
		}
		movimientos = append(movimientos, *movimiento)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inventory movements: %w", err)
	}
	return movimientos, nil
}

// BajoStock lists active products whose total stock is at or below their
// minimum, grouped by principal supplier (RF-04/PR-02). Items without a
// principal supplier go to "Sin proveedor"; inactive suppliers still group
// their items (PR-04).
func (s *Service) BajoStock(ctx context.Context) (*BajoStockReporte, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT p.id, p.codigo, p.nombre, p.stock_minimo,
		        pp.proveedor_id, COALESCE(pr.nombre, ''), COALESCE(pr.activo, false)
		 FROM productos p
		 LEFT JOIN producto_proveedores pp
		        ON pp.producto_id = p.id AND pp.es_principal
		 LEFT JOIN proveedores pr ON pr.id = pp.proveedor_id
		 WHERE p.activo
		 ORDER BY p.codigo, p.id`)
	if err != nil {
		return nil, fmt.Errorf("load low-stock products: %w", err)
	}

	type productoProveedor struct {
		producto       BajoStockProducto
		proveedorID    *int64
		proveedorName  string
		proveedorActiv bool
	}
	candidates := make([]productoProveedor, 0)
	for rows.Next() {
		var candidate productoProveedor
		if err := rows.Scan(&candidate.producto.ProductoID, &candidate.producto.Codigo,
			&candidate.producto.Nombre, &candidate.producto.StockMinimo,
			&candidate.proveedorID, &candidate.proveedorName, &candidate.proveedorActiv); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan low-stock product: %w", err)
		}
		candidate.producto.Depositos = []StockDeposito{}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate low-stock products: %w", err)
	}
	rows.Close()

	totals := make(map[int64]int64)
	totalRows, err := s.pool.Query(ctx,
		`SELECT producto_id, SUM(cantidad) FROM stock_depositos GROUP BY producto_id`)
	if err != nil {
		return nil, fmt.Errorf("load stock totals: %w", err)
	}
	for totalRows.Next() {
		var productoID, total int64
		if err := totalRows.Scan(&productoID, &total); err != nil {
			totalRows.Close()
			return nil, fmt.Errorf("scan stock total: %w", err)
		}
		totals[productoID] = total
	}
	if err := totalRows.Err(); err != nil {
		totalRows.Close()
		return nil, fmt.Errorf("iterate stock totals: %w", err)
	}
	totalRows.Close()

	depositos := make(map[int64][]StockDeposito)
	depositRows, err := s.pool.Query(ctx,
		`SELECT sd.producto_id, sd.deposito_id, d.nombre, sd.cantidad
		 FROM stock_depositos sd
		 JOIN depositos d ON d.id = sd.deposito_id
		 ORDER BY sd.producto_id, d.nombre, sd.deposito_id`)
	if err != nil {
		return nil, fmt.Errorf("load stock breakdown: %w", err)
	}
	for depositRows.Next() {
		var productoID int64
		var row StockDeposito
		if err := depositRows.Scan(&productoID, &row.DepositoID, &row.DepositoNombre, &row.Cantidad); err != nil {
			depositRows.Close()
			return nil, fmt.Errorf("scan stock breakdown: %w", err)
		}
		depositos[productoID] = append(depositos[productoID], row)
	}
	if err := depositRows.Err(); err != nil {
		depositRows.Close()
		return nil, fmt.Errorf("iterate stock breakdown: %w", err)
	}
	depositRows.Close()

	grupos := make(map[int64]*BajoStockGrupo)
	for _, candidate := range candidates {
		total := totals[candidate.producto.ProductoID]
		if total > candidate.producto.StockMinimo {
			continue
		}
		candidate.producto.StockTotal = total
		if breakdown, ok := depositos[candidate.producto.ProductoID]; ok {
			candidate.producto.Depositos = breakdown
		}

		key := int64(0)
		if candidate.proveedorID != nil {
			key = *candidate.proveedorID
		}
		grupo, ok := grupos[key]
		if !ok {
			grupo = &BajoStockGrupo{
				ProveedorID:     candidate.proveedorID,
				ProveedorNombre: candidate.proveedorName,
				Productos:       []BajoStockProducto{},
			}
			if candidate.proveedorID == nil {
				grupo.ProveedorNombre = "Sin proveedor"
			} else {
				activo := candidate.proveedorActiv
				grupo.ProveedorActivo = &activo
			}
			grupos[key] = grupo
		}
		grupo.Productos = append(grupo.Productos, candidate.producto)
	}

	result := make([]BajoStockGrupo, 0, len(grupos))
	for _, grupo := range grupos {
		result = append(result, *grupo)
	}
	sort.Slice(result, func(i, j int) bool {
		if (result[i].ProveedorID == nil) != (result[j].ProveedorID == nil) {
			return result[j].ProveedorID == nil
		}
		return strings.ToLower(result[i].ProveedorNombre) < strings.ToLower(result[j].ProveedorNombre)
	})
	return &BajoStockReporte{Grupos: result}, nil
}
