package pos

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

type recetaHeader struct {
	ID            int64
	ProductoID    int64
	Rendimiento   int64
	UsuarioID     int64
	CreadoEn      time.Time
	ActualizadoEn time.Time
}

const recetaHeaderColumns = `id, producto_elaborado_id, rendimiento, usuario_id,
	creado_en, actualizado_en`

func scanRecetaHeader(row pgx.Row) (*recetaHeader, error) {
	var header recetaHeader
	if err := row.Scan(&header.ID, &header.ProductoID, &header.Rendimiento,
		&header.UsuarioID, &header.CreadoEn, &header.ActualizadoEn); err != nil {
		return nil, err
	}
	return &header, nil
}

// loadRecetaHeader returns nil (not an error) when the product has no recipe.
func loadRecetaHeader(ctx context.Context, q dbQuerier, productoID int64, forUpdate bool) (*recetaHeader, error) {
	query := `SELECT ` + recetaHeaderColumns + ` FROM recetas WHERE producto_elaborado_id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	header, err := scanRecetaHeader(q.QueryRow(ctx, query, productoID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load recipe header: %w", err)
	}
	return header, nil
}

func loadRecetaItems(ctx context.Context, q dbQuerier, recetaID int64) ([]RecetaItem, error) {
	rows, err := q.Query(ctx,
		`SELECT d.producto_insumo_id, p.codigo, p.nombre, p.unidad, d.cantidad
		 FROM receta_detalles d
		 JOIN productos p ON p.id = d.producto_insumo_id
		 WHERE d.receta_id = $1
		 ORDER BY p.codigo, p.id`, recetaID,
	)
	if err != nil {
		return nil, fmt.Errorf("load recipe items: %w", err)
	}
	defer rows.Close()

	items := make([]RecetaItem, 0)
	for rows.Next() {
		var item RecetaItem
		if err := rows.Scan(&item.IngredienteID, &item.Codigo, &item.Nombre,
			&item.Unidad, &item.Cantidad); err != nil {
			return nil, fmt.Errorf("scan recipe item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recipe items: %w", err)
	}
	return items, nil
}

func recetaFromHeader(header *recetaHeader, items []RecetaItem) *Receta {
	receta := &Receta{
		ProductoID:  header.ProductoID,
		Rendimiento: header.Rendimiento,
		Items:       items,
	}
	creado := header.CreadoEn
	actualizado := header.ActualizadoEn
	receta.CreadoEn = &creado
	receta.ActualizadoEn = &actualizado
	return receta
}

// GetReceta returns the current recipe. A product without a recipe is not an
// error: it is returned with rendimiento 1 and an empty item list.
func (s *Service) GetReceta(ctx context.Context, productoID int64) (*Receta, error) {
	if _, err := loadProducto(ctx, s.pool, productoID); err != nil {
		return nil, err
	}
	header, err := loadRecetaHeader(ctx, s.pool, productoID, false)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return &Receta{ProductoID: productoID, Rendimiento: 1, Items: []RecetaItem{}}, nil
	}
	items, err := loadRecetaItems(ctx, s.pool, header.ID)
	if err != nil {
		return nil, err
	}
	return recetaFromHeader(header, items), nil
}

func normalizeRecetaItems(productoID int64, raw []RecetaItemInput) ([]RecetaItemInput, error) {
	items := make([]RecetaItemInput, 0, len(raw))
	seen := make(map[int64]bool, len(raw))
	for _, item := range raw {
		if item.IngredienteID <= 0 {
			return nil, ErrIngredienteNoExiste
		}
		if item.IngredienteID == productoID {
			return nil, ErrRecetaAutoingrediente
		}
		if item.Cantidad <= 0 {
			return nil, ErrRecetaCantidad
		}
		if seen[item.IngredienteID] {
			return nil, ErrRecetaIngredienteRepetido
		}
		seen[item.IngredienteID] = true
		items = append(items, item)
	}
	return items, nil
}

func ensureIngredienteActivo(ctx context.Context, q dbQuerier, ingredienteID int64) error {
	var activo bool
	err := q.QueryRow(ctx,
		`SELECT activo FROM productos WHERE id = $1`, ingredienteID,
	).Scan(&activo)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIngredienteNoExiste
	}
	if err != nil {
		return fmt.Errorf("check ingredient: %w", err)
	}
	if !activo {
		return ErrIngredienteInactivo
	}
	return nil
}

// detectarCicloReceta checks that replacing the recipe of productoID does not
// create a path back to it. Edges of productoID are excluded because they are
// exactly the ones being replaced; the proposed edges are traversed from every
// ingredient.
func detectarCicloReceta(ctx context.Context, q dbQuerier, productoID int64, items []RecetaItemInput) error {
	rows, err := q.Query(ctx,
		`SELECT r.producto_elaborado_id, d.producto_insumo_id
		 FROM receta_detalles d
		 JOIN recetas r ON r.id = d.receta_id
		 WHERE r.producto_elaborado_id <> $1`, productoID,
	)
	if err != nil {
		return fmt.Errorf("load recipe graph: %w", err)
	}
	defer rows.Close()

	graph := make(map[int64][]int64)
	for rows.Next() {
		var from, to int64
		if err := rows.Scan(&from, &to); err != nil {
			return fmt.Errorf("scan recipe graph: %w", err)
		}
		graph[from] = append(graph[from], to)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate recipe graph: %w", err)
	}

	for _, item := range items {
		seen := make(map[int64]bool)
		if llegaA(item.IngredienteID, productoID, graph, seen) {
			return ErrRecetaCiclo
		}
	}
	return nil
}

func llegaA(actual, objetivo int64, graph map[int64][]int64, seen map[int64]bool) bool {
	if actual == objetivo {
		return true
	}
	if seen[actual] {
		return false
	}
	seen[actual] = true
	for _, siguiente := range graph[actual] {
		if llegaA(siguiente, objetivo, graph, seen) {
			return true
		}
	}
	return false
}

// UpdateReceta replaces the complete recipe of one product in a single
// transaction (RF-28/PR-03). An empty item list removes it. Every ingredient
// must exist and be active; self-references, duplicates and cycles are rejected
// before the existing recipe is touched (AC3/AC6).
func (s *Service) UpdateReceta(ctx context.Context, productoID int64, in UpdateRecetaInput) (*Receta, error) {
	if productoID <= 0 {
		return nil, ErrProductoNoExiste
	}
	if in.Items == nil {
		return nil, ErrRecetaItemsRequeridos
	}
	items, err := normalizeRecetaItems(productoID, in.Items)
	if err != nil {
		return nil, err
	}
	if in.Rendimiento != nil && *in.Rendimiento <= 0 {
		return nil, ErrRecetaRendimiento
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update recipe: %w", err)
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

	for _, item := range items {
		if err := ensureIngredienteActivo(ctx, tx, item.IngredienteID); err != nil {
			return nil, err
		}
	}
	if err := detectarCicloReceta(ctx, tx, productoID, items); err != nil {
		return nil, err
	}

	header, err := loadRecetaHeader(ctx, tx, productoID, true)
	if err != nil {
		return nil, err
	}

	rendimiento := int64(1)
	if header != nil {
		rendimiento = header.Rendimiento
	}
	if in.Rendimiento != nil {
		rendimiento = *in.Rendimiento
	}

	if len(items) == 0 {
		if header != nil {
			if _, err := tx.Exec(ctx, `DELETE FROM recetas WHERE id = $1`, header.ID); err != nil {
				return nil, fmt.Errorf("delete recipe: %w", err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
				 VALUES ($1, 'receta_eliminada', 'producto', $2,
				         jsonb_build_object('receta_id', $3::bigint))`,
				in.UsuarioID, productoID, header.ID,
			); err != nil {
				return nil, fmt.Errorf("audit recipe removal: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit recipe removal: %w", err)
		}
		return &Receta{ProductoID: productoID, Rendimiento: 1, Items: []RecetaItem{}}, nil
	}

	var recetaID int64
	if header == nil {
		if err := tx.QueryRow(ctx,
			`INSERT INTO recetas (producto_elaborado_id, rendimiento, usuario_id)
			 VALUES ($1, $2, $3) RETURNING id`,
			productoID, rendimiento, in.UsuarioID,
		).Scan(&recetaID); err != nil {
			return nil, fmt.Errorf("insert recipe: %w", err)
		}
	} else {
		recetaID = header.ID
		if _, err := tx.Exec(ctx,
			`UPDATE recetas
			 SET rendimiento = $2, usuario_id = $3, actualizado_en = now()
			 WHERE id = $1`, recetaID, rendimiento, in.UsuarioID,
		); err != nil {
			return nil, fmt.Errorf("update recipe: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM receta_detalles WHERE receta_id = $1`, recetaID,
		); err != nil {
			return nil, fmt.Errorf("clear recipe items: %w", err)
		}
	}

	for _, item := range items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO receta_detalles (receta_id, producto_insumo_id, cantidad)
			 VALUES ($1, $2, $3)`, recetaID, item.IngredienteID, item.Cantidad,
		); err != nil {
			return nil, fmt.Errorf("insert recipe item: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'receta_actualizada', 'producto', $2,
		         jsonb_build_object('receta_id', $3::bigint,
		                            'ingredientes', $4::bigint,
		                            'rendimiento', $5::bigint))`,
		in.UsuarioID, productoID, recetaID, int64(len(items)), rendimiento,
	); err != nil {
		return nil, fmt.Errorf("audit recipe update: %w", err)
	}

	updatedHeader, err := loadRecetaHeader(ctx, tx, productoID, false)
	if err != nil {
		return nil, err
	}
	updatedItems, err := loadRecetaItems(ctx, tx, recetaID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit recipe update: %w", err)
	}
	return recetaFromHeader(updatedHeader, updatedItems), nil
}

// costoLinea multiplies quantity × unit cost with an overflow guard (RT-01).
func costoLinea(cantidad, costoUnitarioBs int64) (int64, error) {
	if cantidad <= 0 || costoUnitarioBs < 0 {
		return 0, ErrMonto
	}
	if costoUnitarioBs != 0 && cantidad > math.MaxInt64/costoUnitarioBs {
		return 0, ErrMonto
	}
	return cantidad * costoUnitarioBs, nil
}

func sumarCosto(actual, adicional int64) (int64, error) {
	if actual < 0 || adicional < 0 || adicional > math.MaxInt64-actual {
		return 0, ErrMonto
	}
	return actual + adicional, nil
}

func appendIngredienteSinCosto(list []IngredienteSinCosto, items ...IngredienteSinCosto) []IngredienteSinCosto {
	for _, item := range items {
		found := false
		for _, existing := range list {
			if existing.ProductoID == item.ProductoID {
				found = true
				break
			}
		}
		if !found {
			list = append(list, item)
		}
	}
	return list
}

// resolveCostoProducto computes the integer cost of a product. A recipe takes
// precedence over the product's own purchase cost; ingredients are resolved
// recursively. costo is nil when any branch lacks both cost and recipe, so the
// caller never confuses "unknown" with zero (AC4/AC5/AC6).
func resolveCostoProducto(ctx context.Context, q dbQuerier, productoID int64,
	stack map[int64]bool) (*int64, string, []IngredienteSinCosto, error) {
	if stack[productoID] {
		return nil, "", nil, ErrRecetaCiclo
	}
	stack[productoID] = true
	defer delete(stack, productoID)

	header, err := loadRecetaHeader(ctx, q, productoID, false)
	if err != nil {
		return nil, "", nil, err
	}
	if header != nil {
		items, err := loadRecetaItems(ctx, q, header.ID)
		if err != nil {
			return nil, "", nil, err
		}
		if len(items) > 0 {
			var total int64
			missing := make([]IngredienteSinCosto, 0)
			for _, item := range items {
				childCost, _, childMissing, err := resolveCostoProducto(ctx, q, item.IngredienteID, stack)
				if err != nil {
					return nil, "", nil, err
				}
				if childCost == nil {
					if len(childMissing) == 0 {
						childMissing = []IngredienteSinCosto{{
							ProductoID: item.IngredienteID,
							Codigo:     item.Codigo,
							Nombre:     item.Nombre,
						}}
					}
					missing = appendIngredienteSinCosto(missing, childMissing...)
					continue
				}
				line, err := costoLinea(item.Cantidad, *childCost)
				if err != nil {
					return nil, "", nil, err
				}
				total, err = sumarCosto(total, line)
				if err != nil {
					return nil, "", nil, err
				}
			}
			if len(missing) > 0 {
				return nil, "receta", missing, nil
			}
			costo := total
			return &costo, "receta", nil, nil
		}
	}

	costo, err := costoVigente(ctx, q, productoID)
	if err != nil {
		return nil, "", nil, err
	}
	if costo == nil {
		producto, err := loadProducto(ctx, q, productoID)
		if err != nil {
			return nil, "", nil, err
		}
		return nil, "costo_compra", []IngredienteSinCosto{{
			ProductoID: producto.ID,
			Codigo:     producto.Codigo,
			Nombre:     producto.Nombre,
		}}, nil
	}
	return &costo.CostoBs, "costo_compra", nil, nil
}

// CostoMargen returns cost and margin for one product. Missing costs are
// reported explicitly as nil plus the ingredient list; the response never
// substitutes zero or fails with an internal error (AC2/AC4/AC6).
func (s *Service) CostoMargen(ctx context.Context, productoID int64) (*CostoMargen, error) {
	producto, err := loadProducto(ctx, s.pool, productoID)
	if err != nil {
		return nil, err
	}

	costo, origen, missing, err := resolveCostoProducto(ctx, s.pool, productoID, make(map[int64]bool))
	if err != nil {
		return nil, err
	}

	result := &CostoMargen{
		ProductoID:           producto.ID,
		PrecioVentaBs:        producto.PrecioVentaBs,
		Origen:               origen,
		CostoCompleto:        costo != nil,
		IngredientesSinCosto: make([]IngredienteSinCosto, 0),
	}
	for _, item := range missing {
		if item.ProductoID == producto.ID {
			continue
		}
		result.IngredientesSinCosto = appendIngredienteSinCosto(result.IngredientesSinCosto, item)
	}
	if costo == nil {
		return result, nil
	}

	margen := producto.PrecioVentaBs - *costo
	result.CostoBs = costo
	result.MargenBs = &margen
	return result, nil
}
