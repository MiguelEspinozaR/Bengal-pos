package pos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxAreaNombreLen = 120

func normalizeAreaNombre(raw string) (string, error) {
	nombre := strings.TrimSpace(raw)
	if nombre == "" {
		return "", ErrAreaNombreRequerido
	}
	if utf8.RuneCountInString(nombre) > maxAreaNombreLen {
		return "", appError("area_nombre_largo",
			fmt.Sprintf("El nombre del área no puede superar %d caracteres", maxAreaNombreLen), 422)
	}
	return nombre, nil
}

func validateAreaOrden(orden int) error {
	if orden < 0 || orden > 1_000_000 {
		return ErrAreaOrdenInvalido
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const areaColumns = `id, nombre, orden_impresion, activo`

func scanArea(row pgx.Row) (*AreaPreparacion, error) {
	var area AreaPreparacion
	if err := row.Scan(&area.ID, &area.Nombre, &area.OrdenImpresion, &area.Activo); err != nil {
		return nil, err
	}
	return &area, nil
}

// ListAreas returns every configured preparation area. Inactive areas are
// included so historical prints and administration forms can show them.
func (s *Service) ListAreas(ctx context.Context) ([]AreaPreparacion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+areaColumns+` FROM areas_preparacion ORDER BY orden_impresion, id`)
	if err != nil {
		return nil, fmt.Errorf("list preparation areas: %w", err)
	}
	defer rows.Close()

	areas := make([]AreaPreparacion, 0)
	for rows.Next() {
		var area AreaPreparacion
		if err := rows.Scan(&area.ID, &area.Nombre, &area.OrdenImpresion, &area.Activo); err != nil {
			return nil, fmt.Errorf("scan preparation area: %w", err)
		}
		areas = append(areas, area)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate preparation areas: %w", err)
	}
	return areas, nil
}

// CreateArea adds a preparation area without code changes (O-05/RF-26).
func (s *Service) CreateArea(ctx context.Context, in CreateAreaInput) (*AreaPreparacion, error) {
	nombre, err := normalizeAreaNombre(in.Nombre)
	if err != nil {
		return nil, err
	}
	if err := validateAreaOrden(in.OrdenImpresion); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create area: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	area, err := scanArea(tx.QueryRow(ctx,
		`INSERT INTO areas_preparacion (nombre, orden_impresion)
		 VALUES ($1, $2)
		 RETURNING `+areaColumns,
		nombre, in.OrdenImpresion,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAreaNombreDuplicado
		}
		return nil, fmt.Errorf("insert preparation area: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create area: %w", err)
	}
	return area, nil
}

// UpdateArea renames, reorders or (de)activates an area. Records are never
// deleted (RT-06).
func (s *Service) UpdateArea(ctx context.Context, areaID int64, in UpdateAreaInput) (*AreaPreparacion, error) {
	if areaID <= 0 {
		return nil, ErrAreaNoExiste
	}
	var nombre *string
	if in.Nombre != nil {
		normalized, err := normalizeAreaNombre(*in.Nombre)
		if err != nil {
			return nil, err
		}
		nombre = &normalized
	}
	if in.OrdenImpresion != nil {
		if err := validateAreaOrden(*in.OrdenImpresion); err != nil {
			return nil, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update area: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	current, err := scanArea(tx.QueryRow(ctx,
		`SELECT `+areaColumns+` FROM areas_preparacion WHERE id = $1 FOR UPDATE`, areaID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAreaNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("load preparation area: %w", err)
	}

	if nombre != nil {
		current.Nombre = *nombre
	}
	if in.OrdenImpresion != nil {
		current.OrdenImpresion = *in.OrdenImpresion
	}
	if in.Activo != nil {
		current.Activo = *in.Activo
	}

	area, err := scanArea(tx.QueryRow(ctx,
		`UPDATE areas_preparacion
		 SET nombre = $2, orden_impresion = $3, activo = $4
		 WHERE id = $1
		 RETURNING `+areaColumns,
		areaID, current.Nombre, current.OrdenImpresion, current.Activo,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAreaNombreDuplicado
		}
		return nil, fmt.Errorf("update preparation area: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update area: %w", err)
	}
	return area, nil
}

// AssignProductArea sets or clears productos.area_preparacion_id, which is the
// source used to split comandas. An inactive area cannot be assigned to new
// items (RT-06).
func (s *Service) AssignProductArea(ctx context.Context, productoID int64, in AssignProductAreaInput) (*ProductoArea, error) {
	if productoID <= 0 {
		return nil, ErrProductoNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin assign product area: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	if in.AreaPreparacionID != nil {
		var activo bool
		err := tx.QueryRow(ctx,
			`SELECT activo FROM areas_preparacion WHERE id = $1`, *in.AreaPreparacionID,
		).Scan(&activo)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAreaNoExiste
		}
		if err != nil {
			return nil, fmt.Errorf("load preparation area: %w", err)
		}
		if !activo {
			return nil, ErrAreaInactiva
		}
	}

	var product ProductoArea
	err = tx.QueryRow(ctx,
		`UPDATE productos
		 SET area_preparacion_id = $2, actualizado_en = now()
		 WHERE id = $1
		 RETURNING id, codigo, nombre, area_preparacion_id`,
		productoID, in.AreaPreparacionID,
	).Scan(&product.ProductoID, &product.Codigo, &product.Nombre, &product.AreaPreparacionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductoNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("update product area: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit assign product area: %w", err)
	}
	return &product, nil
}
