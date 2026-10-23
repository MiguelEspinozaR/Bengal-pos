package pos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const maxCatalogoNombreLen = 160

// canManageCatalogo allows dueño, administrador and encargado to maintain the
// product/deposit catalog and operate inventory. Empleados only sell (RF-11).
func canManageCatalogo(rol string) bool {
	return rol == "dueno" || rol == "administrador" || rol == "encargado"
}

func normalizeRequiredText(raw string, requiredErr *AppError) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", requiredErr
	}
	if utf8.RuneCountInString(value) > maxCatalogoNombreLen {
		return "", appError(requiredErr.Code+"_largo",
			fmt.Sprintf("El texto no puede superar %d caracteres", maxCatalogoNombreLen), 422)
	}
	return value, nil
}

func normalizeOptionalText(raw *string) *string {
	if raw == nil {
		return nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil
	}
	return &value
}

func normalizedUnidad(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "unidad"
	}
	return value
}

const productoColumns = `id, codigo, nombre, categoria_id, area_preparacion_id, unidad,
	stock_minimo, precio_venta_bs, activo, creado_en, actualizado_en`

func scanProducto(row pgx.Row) (*Producto, error) {
	var p Producto
	if err := row.Scan(&p.ID, &p.Codigo, &p.Nombre, &p.CategoriaID, &p.AreaPreparacionID,
		&p.Unidad, &p.StockMinimo, &p.PrecioVentaBs, &p.Activo, &p.CreadoEn, &p.ActualizadoEn); err != nil {
		return nil, err
	}
	return &p, nil
}

func loadProducto(ctx context.Context, q dbQuerier, productoID int64) (*Producto, error) {
	producto, err := scanProducto(q.QueryRow(ctx,
		`SELECT `+productoColumns+` FROM productos WHERE id = $1`, productoID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductoNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load product: %w", err)
	}
	return producto, nil
}

func validatePrecioStock(stockMinimo, precio int64) error {
	if stockMinimo < 0 {
		return ErrStockMinimoInvalido
	}
	if precio < 0 {
		return ErrPrecioProductoInvalido
	}
	return nil
}

func ensureCategoria(ctx context.Context, q dbQuerier, categoriaID *int64) error {
	if categoriaID == nil {
		return nil
	}
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM categorias WHERE id = $1)`, *categoriaID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check category: %w", err)
	}
	if !exists {
		return ErrCategoriaNoExiste
	}
	return nil
}

// CreateProducto registers a product with a unique, trimmed code (RF-01/PR-01).
func (s *Service) CreateProducto(ctx context.Context, in CreateProductoInput) (*Producto, error) {
	codigo, err := normalizeRequiredText(in.Codigo, ErrCodigoProductoRequerido)
	if err != nil {
		return nil, err
	}
	nombre, err := normalizeRequiredText(in.Nombre, ErrNombreProductoRequerido)
	if err != nil {
		return nil, err
	}
	if err := validatePrecioStock(in.StockMinimo, in.PrecioVentaBs); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create product: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canManageCatalogo(rol) {
		return nil, ErrNoAutorizado
	}
	if err := ensureCategoria(ctx, tx, in.CategoriaID); err != nil {
		return nil, err
	}

	producto, err := scanProducto(tx.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, categoria_id, unidad, stock_minimo, precio_venta_bs)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+productoColumns,
		codigo, nombre, in.CategoriaID, normalizedUnidad(in.Unidad), in.StockMinimo, in.PrecioVentaBs,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCodigoProductoDuplicado
		}
		return nil, fmt.Errorf("insert product: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'producto_creado', 'producto', $2,
		         jsonb_build_object('codigo', $3::text, 'nombre', $4::text))`,
		in.UsuarioID, producto.ID, producto.Codigo, producto.Nombre,
	); err != nil {
		return nil, fmt.Errorf("audit product creation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create product: %w", err)
	}
	return producto, nil
}

// ListProductos returns active products by default; inactive rows are kept for
// history and exposed only when explicitly requested (PR-03/RT-06).
func (s *Service) ListProductos(ctx context.Context, incluirInactivos bool) ([]Producto, error) {
	query := `SELECT ` + productoColumns + ` FROM productos`
	args := []any{}
	if !incluirInactivos {
		query += ` WHERE activo`
	}
	query += ` ORDER BY codigo, id`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	productos := make([]Producto, 0)
	for rows.Next() {
		producto, err := scanProducto(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		productos = append(productos, *producto)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return productos, nil
}

// GetProducto returns one product even when it is inactive.
func (s *Service) GetProducto(ctx context.Context, productoID int64) (*Producto, error) {
	return loadProducto(ctx, s.pool, productoID)
}

// UpdateProducto edits catalog fields or deactivates a product. There is no
// physical delete and stock is never edited directly here (AC2/AC12).
func (s *Service) UpdateProducto(ctx context.Context, productoID int64, in UpdateProductoInput) (*Producto, error) {
	if productoID <= 0 {
		return nil, ErrProductoNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update product: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canManageCatalogo(rol) {
		return nil, ErrNoAutorizado
	}

	current, err := scanProducto(tx.QueryRow(ctx,
		`SELECT `+productoColumns+` FROM productos WHERE id = $1 FOR UPDATE`, productoID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductoNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load product for update: %w", err)
	}

	if in.Codigo != nil {
		codigo, err := normalizeRequiredText(*in.Codigo, ErrCodigoProductoRequerido)
		if err != nil {
			return nil, err
		}
		current.Codigo = codigo
	}
	if in.Nombre != nil {
		nombre, err := normalizeRequiredText(*in.Nombre, ErrNombreProductoRequerido)
		if err != nil {
			return nil, err
		}
		current.Nombre = nombre
	}
	if in.Unidad != nil {
		current.Unidad = normalizedUnidad(*in.Unidad)
	}
	if in.StockMinimo != nil {
		if *in.StockMinimo < 0 {
			return nil, ErrStockMinimoInvalido
		}
		current.StockMinimo = *in.StockMinimo
	}
	if in.PrecioVentaBs != nil {
		if *in.PrecioVentaBs < 0 {
			return nil, ErrPrecioProductoInvalido
		}
		current.PrecioVentaBs = *in.PrecioVentaBs
	}
	if in.CategoriaID != nil {
		if err := ensureCategoria(ctx, tx, in.CategoriaID); err != nil {
			return nil, err
		}
		current.CategoriaID = in.CategoriaID
	}
	if in.Activo != nil {
		current.Activo = *in.Activo
	}

	producto, err := scanProducto(tx.QueryRow(ctx,
		`UPDATE productos
		 SET codigo = $2, nombre = $3, categoria_id = $4, unidad = $5,
		     stock_minimo = $6, precio_venta_bs = $7, activo = $8, actualizado_en = now()
		 WHERE id = $1
		 RETURNING `+productoColumns,
		productoID, current.Codigo, current.Nombre, current.CategoriaID, current.Unidad,
		current.StockMinimo, current.PrecioVentaBs, current.Activo,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCodigoProductoDuplicado
		}
		return nil, fmt.Errorf("update product: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'producto_actualizado', 'producto', $2,
		         jsonb_build_object('codigo', $3::text, 'activo', $4::boolean))`,
		in.UsuarioID, producto.ID, producto.Codigo, producto.Activo,
	); err != nil {
		return nil, fmt.Errorf("audit product update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update product: %w", err)
	}
	return producto, nil
}

const depositoColumns = `id, nombre, ubicacion, activo, creado_en, actualizado_en`

func scanDeposito(row pgx.Row) (*Deposito, error) {
	var d Deposito
	if err := row.Scan(&d.ID, &d.Nombre, &d.Ubicacion, &d.Activo, &d.CreadoEn, &d.ActualizadoEn); err != nil {
		return nil, err
	}
	return &d, nil
}

func loadDeposito(ctx context.Context, q dbQuerier, depositoID int64) (*Deposito, error) {
	deposito, err := scanDeposito(q.QueryRow(ctx,
		`SELECT `+depositoColumns+` FROM depositos WHERE id = $1`, depositoID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDepositoNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load deposit: %w", err)
	}
	return deposito, nil
}

// CreateDeposito registers a storage location with a unique name (RF-05).
func (s *Service) CreateDeposito(ctx context.Context, in CreateDepositoInput) (*Deposito, error) {
	nombre, err := normalizeRequiredText(in.Nombre, ErrDepositoNombreRequerido)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create deposit: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canManageCatalogo(rol) {
		return nil, ErrNoAutorizado
	}

	deposito, err := scanDeposito(tx.QueryRow(ctx,
		`INSERT INTO depositos (nombre, ubicacion)
		 VALUES ($1, $2)
		 RETURNING `+depositoColumns,
		nombre, normalizeOptionalText(in.Ubicacion),
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDepositoNombreDuplicado
		}
		return nil, fmt.Errorf("insert deposit: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'deposito_creado', 'deposito', $2,
		         jsonb_build_object('nombre', $3::text))`,
		in.UsuarioID, deposito.ID, deposito.Nombre,
	); err != nil {
		return nil, fmt.Errorf("audit deposit creation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create deposit: %w", err)
	}
	return deposito, nil
}

// ListDepositos returns active deposits by default; inactive ones are kept for
// history and their stock stays queryable (AC3/RT-06).
func (s *Service) ListDepositos(ctx context.Context, incluirInactivos bool) ([]Deposito, error) {
	query := `SELECT ` + depositoColumns + ` FROM depositos`
	if !incluirInactivos {
		query += ` WHERE activo`
	}
	query += ` ORDER BY nombre, id`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list deposits: %w", err)
	}
	defer rows.Close()

	depositos := make([]Deposito, 0)
	for rows.Next() {
		deposito, err := scanDeposito(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deposit: %w", err)
		}
		depositos = append(depositos, *deposito)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deposits: %w", err)
	}
	return depositos, nil
}

// UpdateDeposito edits or deactivates a deposit without deleting it (AC3).
func (s *Service) UpdateDeposito(ctx context.Context, depositoID int64, in UpdateDepositoInput) (*Deposito, error) {
	if depositoID <= 0 {
		return nil, ErrDepositoNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update deposit: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canManageCatalogo(rol) {
		return nil, ErrNoAutorizado
	}

	current, err := scanDeposito(tx.QueryRow(ctx,
		`SELECT `+depositoColumns+` FROM depositos WHERE id = $1 FOR UPDATE`, depositoID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDepositoNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load deposit for update: %w", err)
	}

	if in.Nombre != nil {
		nombre, err := normalizeRequiredText(*in.Nombre, ErrDepositoNombreRequerido)
		if err != nil {
			return nil, err
		}
		current.Nombre = nombre
	}
	if in.Ubicacion != nil {
		current.Ubicacion = normalizeOptionalText(in.Ubicacion)
	}
	if in.Activo != nil {
		current.Activo = *in.Activo
	}

	deposito, err := scanDeposito(tx.QueryRow(ctx,
		`UPDATE depositos
		 SET nombre = $2, ubicacion = $3, activo = $4, actualizado_en = now()
		 WHERE id = $1
		 RETURNING `+depositoColumns,
		depositoID, current.Nombre, current.Ubicacion, current.Activo,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDepositoNombreDuplicado
		}
		return nil, fmt.Errorf("update deposit: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'deposito_actualizado', 'deposito', $2,
		         jsonb_build_object('nombre', $3::text, 'activo', $4::boolean))`,
		in.UsuarioID, deposito.ID, deposito.Nombre, deposito.Activo,
	); err != nil {
		return nil, fmt.Errorf("audit deposit update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update deposit: %w", err)
	}
	return deposito, nil
}
