package pos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const fechaLayout = "2006-01-02"

// parseFecha validates the ISO date accepted by the cost endpoints.
func parseFecha(raw string) (time.Time, error) {
	fecha, err := time.Parse(fechaLayout, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, ErrCostoFechaInvalida
	}
	return fecha, nil
}

const costoCompraColumns = `c.id, c.producto_id, c.proveedor_id,
	COALESCE(pr.nombre, ''), c.costo_bs, c.fecha, c.usuario_id, c.creado_en`

const costoCompraSelect = `SELECT ` + costoCompraColumns + `
	FROM costos_compra c
	LEFT JOIN proveedores pr ON pr.id = c.proveedor_id`

func scanCostoCompra(row pgx.Row) (*CostoCompra, error) {
	var costo CostoCompra
	var fecha time.Time
	if err := row.Scan(&costo.ID, &costo.ProductoID, &costo.ProveedorID,
		&costo.ProveedorNombre, &costo.CostoBs, &fecha, &costo.UsuarioID,
		&costo.CreadoEn); err != nil {
		return nil, err
	}
	costo.Fecha = fecha.Format(fechaLayout)
	return &costo, nil
}

func loadCostoCompra(ctx context.Context, q dbQuerier, costoID int64) (*CostoCompra, error) {
	costo, err := scanCostoCompra(q.QueryRow(ctx,
		costoCompraSelect+` WHERE c.id = $1`, costoID,
	))
	if err != nil {
		return nil, fmt.Errorf("load purchase cost: %w", err)
	}
	return costo, nil
}

// costoVigente returns the latest purchase cost of a product, or nil when it
// has none. Ties on the same date are resolved by creado_en and id (AC2).
func costoVigente(ctx context.Context, q dbQuerier, productoID int64) (*CostoCompra, error) {
	costo, err := scanCostoCompra(q.QueryRow(ctx,
		costoCompraSelect+`
		WHERE c.producto_id = $1
		ORDER BY c.fecha DESC, c.creado_en DESC, c.id DESC
		LIMIT 1`, productoID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load current purchase cost: %w", err)
	}
	return costo, nil
}

// RegistrarCostoCompra appends one immutable purchase-cost row (RF-27). Only
// administrador/dueño may write; the row is rejected unless the product exists
// and an optional supplier exists and is active.
func (s *Service) RegistrarCostoCompra(ctx context.Context, productoID int64, in CostoCompraInput) (*CostoCompra, error) {
	if productoID <= 0 {
		return nil, ErrProductoNoExiste
	}
	if in.CostoBs < 0 {
		return nil, ErrCostoInvalido
	}
	fecha, err := parseFecha(in.Fecha)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin register purchase cost: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}
	if err := ensureProductoExists(ctx, tx, productoID); err != nil {
		return nil, err
	}
	if in.ProveedorID != nil {
		if *in.ProveedorID <= 0 {
			return nil, ErrProveedorNoExiste
		}
		proveedor, err := loadProveedor(ctx, tx, *in.ProveedorID)
		if err != nil {
			return nil, err
		}
		if !proveedor.Activo {
			return nil, ErrProveedorInactivo
		}
	}

	var costoID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO costos_compra
		 (producto_id, proveedor_id, costo_bs, fecha, usuario_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		productoID, in.ProveedorID, in.CostoBs, fecha, in.UsuarioID,
	).Scan(&costoID); err != nil {
		return nil, fmt.Errorf("insert purchase cost: %w", err)
	}
	costo, err := loadCostoCompra(ctx, tx, costoID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'costo_compra_registrado', 'producto', $2,
		         jsonb_build_object('costo_compra_id', $3::bigint,
		                            'costo_bs', $4::bigint, 'fecha', $5::text))`,
		in.UsuarioID, productoID, costo.ID, costo.CostoBs, costo.Fecha,
	); err != nil {
		return nil, fmt.Errorf("audit purchase cost: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit purchase cost: %w", err)
	}
	return costo, nil
}

// ListCostosProducto returns the read-only cost history, most recent first.
func (s *Service) ListCostosProducto(ctx context.Context, productoID int64) ([]CostoCompra, error) {
	if _, err := loadProducto(ctx, s.pool, productoID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		costoCompraSelect+`
		WHERE c.producto_id = $1
		ORDER BY c.fecha DESC, c.creado_en DESC, c.id DESC`, productoID,
	)
	if err != nil {
		return nil, fmt.Errorf("list purchase costs: %w", err)
	}
	defer rows.Close()

	costos := make([]CostoCompra, 0)
	for rows.Next() {
		costo, err := scanCostoCompra(rows)
		if err != nil {
			return nil, fmt.Errorf("scan purchase cost: %w", err)
		}
		costos = append(costos, *costo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase costs: %w", err)
	}
	return costos, nil
}
