package pos

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const proveedorColumns = `id, nombre, contacto_nombre, telefono, email, direccion,
	activo, creado_en, actualizado_en`

func scanProveedor(row pgx.Row) (*Proveedor, error) {
	var p Proveedor
	if err := row.Scan(&p.ID, &p.Nombre, &p.ContactoNombre, &p.Telefono, &p.Email,
		&p.Direccion, &p.Activo, &p.CreadoEn, &p.ActualizadoEn); err != nil {
		return nil, err
	}
	return &p, nil
}

func loadProveedor(ctx context.Context, q dbQuerier, proveedorID int64) (*Proveedor, error) {
	proveedor, err := scanProveedor(q.QueryRow(ctx,
		`SELECT `+proveedorColumns+` FROM proveedores WHERE id = $1`, proveedorID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProveedorNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load supplier: %w", err)
	}
	return proveedor, nil
}

// CreateProveedor registers a supplier. Only admin/dueño may manage suppliers
// (RF-08/RF-11).
func (s *Service) CreateProveedor(ctx context.Context, in CreateProveedorInput) (*Proveedor, error) {
	nombre, err := normalizeRequiredText(in.Nombre, ErrProveedorNombreRequerido)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create supplier: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	proveedor, err := scanProveedor(tx.QueryRow(ctx,
		`INSERT INTO proveedores (nombre, contacto_nombre, telefono, email, direccion)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+proveedorColumns,
		nombre, normalizeOptionalText(in.ContactoNombre), normalizeOptionalText(in.Telefono),
		normalizeOptionalText(in.Email), normalizeOptionalText(in.Direccion),
	))
	if err != nil {
		return nil, fmt.Errorf("insert supplier: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'proveedor_creado', 'proveedor', $2,
		         jsonb_build_object('nombre', $3::text))`,
		in.UsuarioID, proveedor.ID, proveedor.Nombre,
	); err != nil {
		return nil, fmt.Errorf("audit supplier creation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create supplier: %w", err)
	}
	return proveedor, nil
}

// ListProveedores returns active suppliers by default. Inactive suppliers are
// kept (and still group their items in the low-stock report, PR-04).
func (s *Service) ListProveedores(ctx context.Context, incluirInactivos bool) ([]Proveedor, error) {
	query := `SELECT ` + proveedorColumns + ` FROM proveedores`
	if !incluirInactivos {
		query += ` WHERE activo`
	}
	query += ` ORDER BY nombre, id`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: %w", err)
	}
	defer rows.Close()

	proveedores := make([]Proveedor, 0)
	for rows.Next() {
		proveedor, err := scanProveedor(rows)
		if err != nil {
			return nil, fmt.Errorf("scan supplier: %w", err)
		}
		proveedores = append(proveedores, *proveedor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate suppliers: %w", err)
	}
	return proveedores, nil
}

// UpdateProveedor edits or deactivates a supplier. An inactive supplier can
// only be reactivated; contact edits are rejected until it is active again
// (PR-04). There is no physical delete (RT-06).
func (s *Service) UpdateProveedor(ctx context.Context, proveedorID int64, in UpdateProveedorInput) (*Proveedor, error) {
	if proveedorID <= 0 {
		return nil, ErrProveedorNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update supplier: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	current, err := scanProveedor(tx.QueryRow(ctx,
		`SELECT `+proveedorColumns+` FROM proveedores WHERE id = $1 FOR UPDATE`, proveedorID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProveedorNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load supplier for update: %w", err)
	}

	reactivating := in.Activo != nil && *in.Activo
	if !current.Activo && !reactivating && hasSupplierEdits(in) {
		return nil, ErrProveedorInactivo
	}

	if in.Nombre != nil {
		nombre, err := normalizeRequiredText(*in.Nombre, ErrProveedorNombreRequerido)
		if err != nil {
			return nil, err
		}
		current.Nombre = nombre
	}
	if in.ContactoNombre != nil {
		current.ContactoNombre = normalizeOptionalText(in.ContactoNombre)
	}
	if in.Telefono != nil {
		current.Telefono = normalizeOptionalText(in.Telefono)
	}
	if in.Email != nil {
		current.Email = normalizeOptionalText(in.Email)
	}
	if in.Direccion != nil {
		current.Direccion = normalizeOptionalText(in.Direccion)
	}
	if in.Activo != nil {
		current.Activo = *in.Activo
	}

	proveedor, err := scanProveedor(tx.QueryRow(ctx,
		`UPDATE proveedores
		 SET nombre = $2, contacto_nombre = $3, telefono = $4, email = $5,
		     direccion = $6, activo = $7, actualizado_en = now()
		 WHERE id = $1
		 RETURNING `+proveedorColumns,
		proveedorID, current.Nombre, current.ContactoNombre, current.Telefono,
		current.Email, current.Direccion, current.Activo,
	))
	if err != nil {
		return nil, fmt.Errorf("update supplier: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'proveedor_actualizado', 'proveedor', $2,
		         jsonb_build_object('nombre', $3::text, 'activo', $4::boolean))`,
		in.UsuarioID, proveedor.ID, proveedor.Nombre, proveedor.Activo,
	); err != nil {
		return nil, fmt.Errorf("audit supplier update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update supplier: %w", err)
	}
	return proveedor, nil
}

func hasSupplierEdits(in UpdateProveedorInput) bool {
	return in.Nombre != nil || in.ContactoNombre != nil || in.Telefono != nil ||
		in.Email != nil || in.Direccion != nil
}

const productoProveedorColumns = `pp.producto_id, pp.proveedor_id, pr.nombre,
	pr.contacto_nombre, pr.telefono, pr.email, pr.direccion, pr.activo,
	pp.es_principal, pp.creado_en`

func scanProductoProveedor(row pgx.Row) (*ProductoProveedor, error) {
	var pp ProductoProveedor
	if err := row.Scan(&pp.ProductoID, &pp.ProveedorID, &pp.ProveedorNombre,
		&pp.ContactoNombre, &pp.Telefono, &pp.Email, &pp.Direccion,
		&pp.ProveedorActivo, &pp.EsPrincipal, &pp.CreadoEn); err != nil {
		return nil, err
	}
	return &pp, nil
}

func loadProductoProveedor(ctx context.Context, q dbQuerier, productoID, proveedorID int64) (*ProductoProveedor, error) {
	pp, err := scanProductoProveedor(q.QueryRow(ctx,
		`SELECT `+productoProveedorColumns+`
		 FROM producto_proveedores pp
		 JOIN proveedores pr ON pr.id = pp.proveedor_id
		 WHERE pp.producto_id = $1 AND pp.proveedor_id = $2`,
		productoID, proveedorID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAsociacionNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load item-supplier association: %w", err)
	}
	return pp, nil
}

// ListProductoProveedores returns every supplier associated with a product,
// including inactive suppliers and their preserved associations (RF-07/PR-04).
func (s *Service) ListProductoProveedores(ctx context.Context, productoID int64) ([]ProductoProveedor, error) {
	if _, err := loadProducto(ctx, s.pool, productoID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+productoProveedorColumns+`
		 FROM producto_proveedores pp
		 JOIN proveedores pr ON pr.id = pp.proveedor_id
		 WHERE pp.producto_id = $1
		 ORDER BY pp.es_principal DESC, pr.nombre, pp.proveedor_id`, productoID)
	if err != nil {
		return nil, fmt.Errorf("list item-supplier associations: %w", err)
	}
	defer rows.Close()

	asociaciones := make([]ProductoProveedor, 0)
	for rows.Next() {
		pp, err := scanProductoProveedor(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item-supplier association: %w", err)
		}
		asociaciones = append(asociaciones, *pp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item-supplier associations: %w", err)
	}
	return asociaciones, nil
}

// AsociarProductoProveedor associates an active item with an active supplier.
// Marking a new principal unsets the previous one in the same transaction
// (RF-07/AC5).
func (s *Service) AsociarProductoProveedor(ctx context.Context, productoID int64, in AsociarProveedorInput) (*ProductoProveedor, error) {
	if productoID <= 0 {
		return nil, ErrProductoNoExiste
	}
	if in.ProveedorID <= 0 {
		return nil, ErrProveedorNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin associate supplier: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	var productoActivo bool
	err = tx.QueryRow(ctx, `SELECT activo FROM productos WHERE id = $1 FOR UPDATE`, productoID).Scan(&productoActivo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductoNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load product for association: %w", err)
	}
	if !productoActivo {
		return nil, ErrProductoInactivo
	}

	proveedor, err := loadProveedor(ctx, tx, in.ProveedorID)
	if err != nil {
		return nil, err
	}
	if !proveedor.Activo {
		return nil, ErrProveedorInactivo
	}

	if in.EsPrincipal {
		if _, err := tx.Exec(ctx,
			`UPDATE producto_proveedores SET es_principal = false
			 WHERE producto_id = $1 AND es_principal`, productoID,
		); err != nil {
			return nil, fmt.Errorf("unset previous principal supplier: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO producto_proveedores (producto_id, proveedor_id, es_principal)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (producto_id, proveedor_id)
		 DO UPDATE SET es_principal = EXCLUDED.es_principal`,
		productoID, in.ProveedorID, in.EsPrincipal,
	); err != nil {
		return nil, fmt.Errorf("insert item-supplier association: %w", err)
	}

	pp, err := loadProductoProveedor(ctx, tx, productoID, in.ProveedorID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'producto_proveedor_asociado', 'producto', $2,
		         jsonb_build_object('proveedor_id', $3::bigint, 'es_principal', $4::boolean))`,
		in.UsuarioID, productoID, in.ProveedorID, in.EsPrincipal,
	); err != nil {
		return nil, fmt.Errorf("audit item-supplier association: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit associate supplier: %w", err)
	}
	return pp, nil
}

// RemoveProductoProveedor removes an association (the relation is not
// history). Products and suppliers themselves are never deleted (AC12).
func (s *Service) RemoveProductoProveedor(ctx context.Context, productoID, proveedorID, usuarioID int64) error {
	if productoID <= 0 {
		return ErrProductoNoExiste
	}
	if proveedorID <= 0 {
		return ErrProveedorNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin remove supplier association: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, usuarioID)
	if err != nil {
		return err
	}
	if !canAuthorize(rol) {
		return ErrNoAutorizado
	}

	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM productos WHERE id = $1)`, productoID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check product for association removal: %w", err)
	}
	if !exists {
		return ErrProductoNoExiste
	}

	tag, err := tx.Exec(ctx,
		`DELETE FROM producto_proveedores WHERE producto_id = $1 AND proveedor_id = $2`,
		productoID, proveedorID,
	)
	if err != nil {
		return fmt.Errorf("remove item-supplier association: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAsociacionNoExiste
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'producto_proveedor_desasociado', 'producto', $2,
		         jsonb_build_object('proveedor_id', $3::bigint))`,
		usuarioID, productoID, proveedorID,
	); err != nil {
		return fmt.Errorf("audit item-supplier removal: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit remove supplier association: %w", err)
	}
	return nil
}
