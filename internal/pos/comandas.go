package pos

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"bengalpos/internal/domain"
)

// loadComandaSource reads the sale lines with their product's preparation area.
// It is the input of the pure domain splitting function (RF-26).
func loadComandaSource(ctx context.Context, q dbQuerier, ventaID int64) ([]domain.ComandaSourceItem, error) {
	rows, err := q.Query(ctx,
		`SELECT d.id, d.producto_id, p.nombre, d.cantidad,
		        p.area_preparacion_id, COALESCE(a.nombre, ''), COALESCE(a.orden_impresion, 0)
		 FROM venta_detalles d
		 JOIN productos p ON p.id = d.producto_id
		 LEFT JOIN areas_preparacion a ON a.id = p.area_preparacion_id
		 WHERE d.venta_id = $1
		 ORDER BY d.id`, ventaID)
	if err != nil {
		return nil, fmt.Errorf("load comanda source: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ComandaSourceItem, 0)
	for rows.Next() {
		var item domain.ComandaSourceItem
		if err := rows.Scan(&item.VentaDetalleID, &item.ProductoID, &item.Nombre, &item.Cantidad,
			&item.AreaID, &item.AreaNombre, &item.AreaOrden); err != nil {
			return nil, fmt.Errorf("scan comanda source: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comanda source: %w", err)
	}
	return items, nil
}

// insertComandas persists one comanda per area in the same transaction that
// completes the sale. Printing remains a separate auditable step, so a print
// failure never rolls the sale back (RT-03).
func insertComandas(ctx context.Context, q dbQuerier, ventaID, usuarioID int64, groups []domain.ComandaGroup) ([]Comanda, error) {
	comandas := make([]Comanda, 0, len(groups))
	for _, group := range groups {
		var comanda Comanda
		err := q.QueryRow(ctx,
			`INSERT INTO comandas (venta_id, area_preparacion_id, usuario_id)
			 VALUES ($1, $2, $3)
			 RETURNING id, venta_id, area_preparacion_id, usuario_id, estado,
			           impresa, copia, impresa_en, creado_en`,
			ventaID, group.AreaID, usuarioID,
		).Scan(&comanda.ID, &comanda.VentaID, &comanda.AreaPreparacionID, &comanda.UsuarioID,
			&comanda.Estado, &comanda.Impresa, &comanda.Copia, &comanda.ImpresaEn, &comanda.CreadoEn)
		if err != nil {
			return nil, fmt.Errorf("insert comanda: %w", err)
		}
		comanda.AreaNombre = group.AreaNombre
		comanda.OrdenImpresion = group.Orden
		comanda.Items = make([]ComandaItem, 0, len(group.Items))
		for _, item := range group.Items {
			var comandaItem ComandaItem
			err := q.QueryRow(ctx,
				`INSERT INTO comanda_items
				 (comanda_id, venta_detalle_id, producto_id, cantidad, nombre)
				 VALUES ($1, $2, $3, $4, $5)
				 RETURNING id, comanda_id, venta_detalle_id, producto_id, cantidad, nombre`,
				comanda.ID, item.VentaDetalleID, item.ProductoID, item.Cantidad, item.Nombre,
			).Scan(&comandaItem.ID, &comandaItem.ComandaID, &comandaItem.VentaDetalleID,
				&comandaItem.ProductoID, &comandaItem.Cantidad, &comandaItem.Nombre)
			if err != nil {
				return nil, fmt.Errorf("insert comanda item: %w", err)
			}
			comanda.Items = append(comanda.Items, comandaItem)
		}
		comandas = append(comandas, comanda)
	}
	return comandas, nil
}

// comandaItemNames returns the product names that had no area and therefore
// generated no ticket. Completion stays successful (T-03 compatibility); the
// caller/UI can warn about them.
func comandaItemNames(items []domain.ComandaSourceItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Nombre)
	}
	return names
}

func loadComandaItems(ctx context.Context, q dbQuerier, comandaID int64) ([]ComandaItem, error) {
	rows, err := q.Query(ctx,
		`SELECT id, comanda_id, venta_detalle_id, producto_id, cantidad, nombre
		 FROM comanda_items
		 WHERE comanda_id = $1
		 ORDER BY id`, comandaID)
	if err != nil {
		return nil, fmt.Errorf("load comanda items: %w", err)
	}
	defer rows.Close()

	items := make([]ComandaItem, 0)
	for rows.Next() {
		var item ComandaItem
		if err := rows.Scan(&item.ID, &item.ComandaID, &item.VentaDetalleID,
			&item.ProductoID, &item.Cantidad, &item.Nombre); err != nil {
			return nil, fmt.Errorf("scan comanda item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comanda items: %w", err)
	}
	return items, nil
}

// loadComandas loads every ticket of a sale ordered by print order. Items are
// loaded in one query to avoid N+1 on the common case (few areas per sale).
func loadComandas(ctx context.Context, q dbQuerier, ventaID int64) ([]Comanda, error) {
	rows, err := q.Query(ctx,
		`SELECT c.id, c.venta_id, c.area_preparacion_id, a.nombre, a.orden_impresion,
		        c.usuario_id, c.estado, c.impresa, c.copia, c.impresa_en, c.creado_en
		 FROM comandas c
		 JOIN areas_preparacion a ON a.id = c.area_preparacion_id
		 WHERE c.venta_id = $1
		 ORDER BY a.orden_impresion, c.area_preparacion_id, c.id`, ventaID)
	if err != nil {
		return nil, fmt.Errorf("load comandas: %w", err)
	}

	comandas := make([]Comanda, 0)
	index := make(map[int64]int)
	for rows.Next() {
		var comanda Comanda
		if err := rows.Scan(&comanda.ID, &comanda.VentaID, &comanda.AreaPreparacionID,
			&comanda.AreaNombre, &comanda.OrdenImpresion, &comanda.UsuarioID,
			&comanda.Estado, &comanda.Impresa, &comanda.Copia, &comanda.ImpresaEn,
			&comanda.CreadoEn); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan comanda: %w", err)
		}
		comanda.Items = []ComandaItem{}
		index[comanda.ID] = len(comandas)
		comandas = append(comandas, comanda)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate comandas: %w", err)
	}
	rows.Close()
	if len(comandas) == 0 {
		return comandas, nil
	}

	ids := make([]int64, 0, len(comandas))
	for _, comanda := range comandas {
		ids = append(ids, comanda.ID)
	}
	itemRows, err := q.Query(ctx,
		`SELECT id, comanda_id, venta_detalle_id, producto_id, cantidad, nombre
		 FROM comanda_items
		 WHERE comanda_id = ANY($1)
		 ORDER BY comanda_id, id`, ids)
	if err != nil {
		return nil, fmt.Errorf("load comanda items batch: %w", err)
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var item ComandaItem
		if err := itemRows.Scan(&item.ID, &item.ComandaID, &item.VentaDetalleID,
			&item.ProductoID, &item.Cantidad, &item.Nombre); err != nil {
			return nil, fmt.Errorf("scan comanda item batch: %w", err)
		}
		if pos, ok := index[item.ComandaID]; ok {
			comandas[pos].Items = append(comandas[pos].Items, item)
		}
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comanda items batch: %w", err)
	}
	return comandas, nil
}

// ListComandas returns the tickets generated for a sale (AC1).
func (s *Service) ListComandas(ctx context.Context, ventaID int64) ([]Comanda, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM ventas WHERE id = $1)`, ventaID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check sale for comandas: %w", err)
	}
	if !exists {
		return nil, ErrVentaNoExiste
	}
	return loadComandas(ctx, s.pool, ventaID)
}

const printFormatHTML = "html"
const printFormatText = "texto"

func normalizePrintFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "html":
		return printFormatHTML, nil
	case "texto", "text", "txt":
		return printFormatText, nil
	default:
		return "", ErrFormatoImpresion
	}
}

func scanComanda(row pgx.Row) (*Comanda, error) {
	var comanda Comanda
	err := row.Scan(&comanda.ID, &comanda.VentaID, &comanda.AreaPreparacionID,
		&comanda.AreaNombre, &comanda.OrdenImpresion, &comanda.UsuarioID, &comanda.Estado,
		&comanda.Impresa, &comanda.Copia, &comanda.ImpresaEn, &comanda.CreadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrComandaNoExiste
	}
	if err != nil {
		return nil, fmt.Errorf("scan comanda: %w", err)
	}
	comanda.Items = []ComandaItem{}
	return &comanda, nil
}

// lockVentaForPrint serializes print audits (and therefore copy numbering) for
// one sale. The lock is held until the enclosing transaction commits.
func lockVentaForPrint(ctx context.Context, tx pgx.Tx, ventaID int64) error {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM ventas WHERE id = $1 FOR UPDATE`, ventaID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrVentaNoExiste
	}
	if err != nil {
		return fmt.Errorf("lock sale for printing: %w", err)
	}
	return nil
}

// registerPrint computes the next copy number and writes the insert-only audit
// row. The caller must hold a lock on the sale row so concurrent prints of the
// same document cannot reuse a copy number.
func registerPrint(ctx context.Context, tx pgx.Tx, documentoTipo string, documentoID, ventaID int64, comandaID *int64, usuarioID int64) (Impresion, error) {
	var numero int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(numero_copia), 0) + 1
		 FROM impresiones
		 WHERE documento_tipo = $1 AND documento_id = $2`,
		documentoTipo, documentoID,
	).Scan(&numero); err != nil {
		return Impresion{}, fmt.Errorf("next copy number: %w", err)
	}

	var impresion Impresion
	err := tx.QueryRow(ctx,
		`INSERT INTO impresiones
		 (documento_tipo, documento_id, venta_id, comanda_id, numero_copia, copia, usuario_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, documento_tipo, documento_id, venta_id, comanda_id,
		           numero_copia, copia, usuario_id, creado_en`,
		documentoTipo, documentoID, ventaID, comandaID, numero, numero > 1, usuarioID,
	).Scan(&impresion.ID, &impresion.DocumentoTipo, &impresion.DocumentoID, &impresion.VentaID,
		&impresion.ComandaID, &impresion.NumeroCopia, &impresion.Copia, &impresion.UsuarioID,
		&impresion.CreadoEn)
	if err != nil {
		return Impresion{}, fmt.Errorf("insert print audit: %w", err)
	}
	return impresion, nil
}

// PrintComanda renders a ticket and records the print (original or copy) with
// user and time (RF-25). Reprints are marked "COPIA" (AC3/AC4).
func (s *Service) PrintComanda(ctx context.Context, comandaID, usuarioID int64, formato string) (*PrintDocument, error) {
	formato, err := normalizePrintFormat(formato)
	if err != nil {
		return nil, err
	}
	if comandaID <= 0 {
		return nil, ErrComandaNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin print comanda: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := userRole(ctx, tx, usuarioID); err != nil {
		return nil, err
	}

	comanda, err := scanComanda(tx.QueryRow(ctx,
		`SELECT c.id, c.venta_id, c.area_preparacion_id, a.nombre, a.orden_impresion,
		        c.usuario_id, c.estado, c.impresa, c.copia, c.impresa_en, c.creado_en
		 FROM comandas c
		 JOIN areas_preparacion a ON a.id = c.area_preparacion_id
		 WHERE c.id = $1`, comandaID,
	))
	if err != nil {
		return nil, err
	}
	items, err := loadComandaItems(ctx, tx, comandaID)
	if err != nil {
		return nil, err
	}
	comanda.Items = items

	if err := lockVentaForPrint(ctx, tx, comanda.VentaID); err != nil {
		return nil, err
	}

	impresion, err := registerPrint(ctx, tx, "comanda", comanda.ID, comanda.VentaID, &comanda.ID, usuarioID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE comandas
		 SET estado = 'impresa', impresa = true, copia = $2, impresa_en = now(), actualizado_en = now()
		 WHERE id = $1`,
		comanda.ID, impresion.Copia,
	); err != nil {
		return nil, fmt.Errorf("mark comanda as printed: %w", err)
	}
	comanda.Impresa = true
	comanda.Copia = impresion.Copia

	contenido := RenderComandaTicket(*comanda, impresion.Copia, impresion.NumeroCopia, formato)
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit print comanda: %w", err)
	}
	return &PrintDocument{Formato: formato, Contenido: contenido, Impresion: impresion}, nil
}

// PrintReceipt renders the sale receipt and records the print (original or
// copy) with user and time (RF-25).
func (s *Service) PrintReceipt(ctx context.Context, ventaID, usuarioID int64, formato string) (*PrintDocument, error) {
	formato, err := normalizePrintFormat(formato)
	if err != nil {
		return nil, err
	}
	if ventaID <= 0 {
		return nil, ErrVentaNoExiste
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin print receipt: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := userRole(ctx, tx, usuarioID); err != nil {
		return nil, err
	}

	sale, err := getSale(ctx, tx, ventaID)
	if err != nil {
		return nil, err
	}
	if sale.Estado != "completada" {
		return nil, ErrVentaNoCompletada
	}

	var atendidoPor string
	if err := tx.QueryRow(ctx,
		`SELECT nombre_completo FROM usuarios WHERE id = $1`, sale.UsuarioID,
	).Scan(&atendidoPor); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("load sale user for receipt: %w", err)
	}

	if err := lockVentaForPrint(ctx, tx, ventaID); err != nil {
		return nil, err
	}
	impresion, err := registerPrint(ctx, tx, "recibo", ventaID, ventaID, nil, usuarioID)
	if err != nil {
		return nil, err
	}
	contenido := RenderReceipt(*sale, atendidoPor, impresion.Copia, impresion.NumeroCopia, formato)
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit print receipt: %w", err)
	}
	return &PrintDocument{Formato: formato, Contenido: contenido, Impresion: impresion}, nil
}
