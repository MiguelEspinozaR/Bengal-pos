package pos

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setupT04 reuses the isolated-schema harness from T-03, which provisions an
// ephemeral PostgreSQL cluster when DATABASE_URL is unset.
func setupT04(t *testing.T) (*Service, *pgxpool.Pool, func()) {
	t.Helper()
	return setupT03(t)
}

type t04Seed struct {
	EmpleadoID       int64
	AdminID          int64
	CocinaID         int64
	BarraID          int64
	ProductoCocinaID int64
	ProductoBarraID  int64
	DepositoID       int64
}

func seedT04(t *testing.T, pool *pgxpool.Pool) t04Seed {
	t.Helper()
	ctx := context.Background()
	var seed t04Seed

	if err := pool.QueryRow(ctx,
		`INSERT INTO usuarios (nombre_completo, login, password_hash, rol)
		 VALUES ('Empleado', 'emp1', 'h', 'empleado') RETURNING id`,
	).Scan(&seed.EmpleadoID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO usuarios (nombre_completo, login, password_hash, rol)
		 VALUES ('Admin', 'admin1', 'h', 'administrador') RETURNING id`,
	).Scan(&seed.AdminID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO areas_preparacion (nombre, orden_impresion) VALUES ('Cocina', 1) RETURNING id`,
	).Scan(&seed.CocinaID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO areas_preparacion (nombre, orden_impresion) VALUES ('Barra', 2) RETURNING id`,
	).Scan(&seed.BarraID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO depositos (nombre) VALUES ('Principal') RETURNING id`,
	).Scan(&seed.DepositoID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, precio_venta_bs, area_preparacion_id)
		 VALUES ('COC-1', 'Hamburguesa', 5000, $1) RETURNING id`, seed.CocinaID,
	).Scan(&seed.ProductoCocinaID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, precio_venta_bs, area_preparacion_id)
		 VALUES ('BAR-1', 'Cerveza', 3000, $1) RETURNING id`, seed.BarraID,
	).Scan(&seed.ProductoBarraID); err != nil {
		t.Fatal(err)
	}
	for _, productoID := range []int64{seed.ProductoCocinaID, seed.ProductoBarraID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO stock_depositos (producto_id, deposito_id, cantidad) VALUES ($1, $2, 100)`,
			productoID, seed.DepositoID,
		); err != nil {
			t.Fatal(err)
		}
	}
	return seed
}

func completeByCard(t *testing.T, svc *Service, usuarioID int64, items []CreateSaleItem) *SaleCompletion {
	t.Helper()
	ctx := context.Background()
	sale, err := svc.CreateSale(ctx, CreateSaleInput{UsuarioID: usuarioID, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    usuarioID,
		MetodoCodigo: "tarjeta",
		MontoBs:      sale.TotalBs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, part.ID, usuarioID); err != nil {
		t.Fatal(err)
	}
	completion, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: usuarioID})
	if err != nil {
		t.Fatal(err)
	}
	return completion
}

func TestT04_GeneratesOneComandaPerArea(t *testing.T) {
	svc, pool, cleanup := setupT04(t)
	defer cleanup()
	seed := seedT04(t, pool)
	ctx := context.Background()

	completion := completeByCard(t, svc, seed.EmpleadoID, []CreateSaleItem{
		{ProductoID: seed.ProductoCocinaID, Cantidad: 2},
		{ProductoID: seed.ProductoBarraID, Cantidad: 1},
	})

	if completion.Venta.Estado != "completada" {
		t.Fatalf("estado = %s", completion.Venta.Estado)
	}
	if len(completion.Comandas) != 2 {
		t.Fatalf("comandas = %d, want 2", len(completion.Comandas))
	}
	if len(completion.SinArea) != 0 {
		t.Fatalf("sin area = %v", completion.SinArea)
	}

	cocina := completion.Comandas[0]
	if cocina.AreaPreparacionID != seed.CocinaID || cocina.AreaNombre != "Cocina" {
		t.Fatalf("first comanda area = %+v", cocina)
	}
	if len(cocina.Items) != 1 || cocina.Items[0].Nombre != "Hamburguesa" || cocina.Items[0].Cantidad != 2 {
		t.Fatalf("kitchen ticket = %+v", cocina.Items)
	}
	barra := completion.Comandas[1]
	if barra.AreaPreparacionID != seed.BarraID || barra.AreaNombre != "Barra" {
		t.Fatalf("second comanda area = %+v", barra)
	}
	if len(barra.Items) != 1 || barra.Items[0].Nombre != "Cerveza" {
		t.Fatalf("bar ticket = %+v", barra.Items)
	}

	var comandas, items int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM comandas WHERE venta_id = $1`, completion.Venta.ID).Scan(&comandas); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM comanda_items WHERE comanda_id = ANY($1)`,
		[]int64{cocina.ID, barra.ID}).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if comandas != 2 || items != 2 {
		t.Fatalf("persisted comandas=%d items=%d", comandas, items)
	}

	loaded, err := svc.ListComandas(ctx, completion.Venta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].AreaNombre != "Cocina" || loaded[1].AreaNombre != "Barra" {
		t.Fatalf("ListComandas = %+v", loaded)
	}

	// Completing an already completed sale must not duplicate comandas.
	retry, err := svc.CompleteSale(ctx, completion.Venta.ID, CompleteSaleInput{UsuarioID: seed.EmpleadoID})
	if err != nil {
		t.Fatal(err)
	}
	if len(retry.Comandas) != 2 {
		t.Fatalf("retry comandas = %d, want 2", len(retry.Comandas))
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM comandas WHERE venta_id = $1`, completion.Venta.ID).Scan(&comandas); err != nil {
		t.Fatal(err)
	}
	if comandas != 2 {
		t.Fatalf("duplicated comandas after retry: %d", comandas)
	}
}

func TestT04_AreasConfigurableAndItemAssignment(t *testing.T) {
	svc, pool, cleanup := setupT04(t)
	defer cleanup()
	seed := seedT04(t, pool)
	ctx := context.Background()

	area, err := svc.CreateArea(ctx, CreateAreaInput{
		UsuarioID:      seed.AdminID,
		Nombre:         "  Postres  ",
		OrdenImpresion: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if area.Nombre != "Postres" || area.OrdenImpresion != 3 || !area.Activo {
		t.Fatalf("created area = %+v", area)
	}

	nuevoNombre := "Postres y café"
	orden := 2
	updated, err := svc.UpdateArea(ctx, area.ID, UpdateAreaInput{
		UsuarioID:      seed.AdminID,
		Nombre:         &nuevoNombre,
		OrdenImpresion: &orden,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Nombre != nuevoNombre || updated.OrdenImpresion != 2 {
		t.Fatalf("updated area = %+v", updated)
	}

	areas, err := svc.ListAreas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(areas) != 3 {
		t.Fatalf("areas = %d, want 3", len(areas))
	}

	// A new area works without code changes: assign it and sell the product.
	var productID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, precio_venta_bs)
		 VALUES ('POS-1', 'Tarta', 2000) RETURNING id`,
	).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO stock_depositos (producto_id, deposito_id, cantidad) VALUES ($1, $2, 10)`,
		productID, seed.DepositoID); err != nil {
		t.Fatal(err)
	}
	assigned, err := svc.AssignProductArea(ctx, productID, AssignProductAreaInput{
		UsuarioID:         seed.AdminID,
		AreaPreparacionID: &area.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if assigned.AreaPreparacionID == nil || *assigned.AreaPreparacionID != area.ID {
		t.Fatalf("assigned product = %+v", assigned)
	}

	completion := completeByCard(t, svc, seed.EmpleadoID, []CreateSaleItem{
		{ProductoID: productID, Cantidad: 1},
	})
	if len(completion.Comandas) != 1 || completion.Comandas[0].AreaNombre != nuevoNombre {
		t.Fatalf("comanda for new area = %+v", completion.Comandas)
	}

	// Duplicate names are rejected and inactive areas cannot be assigned.
	if _, err := svc.CreateArea(ctx, CreateAreaInput{
		UsuarioID: seed.AdminID, Nombre: nuevoNombre, OrdenImpresion: 4,
	}); !errors.Is(err, ErrAreaNombreDuplicado) {
		t.Fatalf("duplicate area error = %v", err)
	}
	activo := false
	if _, err := svc.UpdateArea(ctx, area.ID, UpdateAreaInput{UsuarioID: seed.AdminID, Activo: &activo}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignProductArea(ctx, productID, AssignProductAreaInput{
		UsuarioID: seed.AdminID, AreaPreparacionID: &area.ID,
	}); !errors.Is(err, ErrAreaInactiva) {
		t.Fatalf("inactive area assignment error = %v", err)
	}
}

func TestT04_ReprintAuditsCopyUserAndTime(t *testing.T) {
	svc, pool, cleanup := setupT04(t)
	defer cleanup()
	seed := seedT04(t, pool)
	ctx := context.Background()

	completion := completeByCard(t, svc, seed.EmpleadoID, []CreateSaleItem{
		{ProductoID: seed.ProductoCocinaID, Cantidad: 1},
	})
	comandaID := completion.Comandas[0].ID

	original, err := svc.PrintComanda(ctx, comandaID, seed.EmpleadoID, "texto")
	if err != nil {
		t.Fatal(err)
	}
	if original.Impresion.Copia || original.Impresion.NumeroCopia != 1 {
		t.Fatalf("original print = %+v", original.Impresion)
	}
	if strings.Contains(original.Contenido, "COPIA") {
		t.Fatalf("original ticket marked as copy: %s", original.Contenido)
	}

	copia, err := svc.PrintComanda(ctx, comandaID, seed.EmpleadoID, "texto")
	if err != nil {
		t.Fatal(err)
	}
	if !copia.Impresion.Copia || copia.Impresion.NumeroCopia != 2 {
		t.Fatalf("copy print = %+v", copia.Impresion)
	}
	if !strings.Contains(copia.Contenido, "COPIA") {
		t.Fatalf("copy ticket lacks marker: %s", copia.Contenido)
	}

	var auditCount int64
	var auditUser int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*), min(usuario_id) FROM impresiones
		 WHERE documento_tipo = 'comanda' AND documento_id = $1`, comandaID,
	).Scan(&auditCount, &auditUser); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 || auditUser != seed.EmpleadoID {
		t.Fatalf("comanda audit count=%d user=%d", auditCount, auditUser)
	}
	var copiaFlag bool
	if err := pool.QueryRow(ctx,
		`SELECT copia FROM impresiones WHERE id = $1`, copia.Impresion.ID,
	).Scan(&copiaFlag); err != nil {
		t.Fatal(err)
	}
	if !copiaFlag {
		t.Fatal("reprint audit row is not marked as copy")
	}
	// Impresiones is insert-only history (RNF-03).
	if _, err := pool.Exec(ctx, `UPDATE impresiones SET copia = false WHERE id = $1`, copia.Impresion.ID); err == nil {
		t.Fatal("UPDATE impresiones unexpectedly succeeded")
	}

	receiptOriginal, err := svc.PrintReceipt(ctx, completion.Venta.ID, seed.EmpleadoID, "texto")
	if err != nil {
		t.Fatal(err)
	}
	if receiptOriginal.Impresion.Copia || !strings.Contains(receiptOriginal.Contenido, "RECIBO DE VENTA") {
		t.Fatalf("original receipt = %+v", receiptOriginal)
	}
	receiptCopy, err := svc.PrintReceipt(ctx, completion.Venta.ID, seed.EmpleadoID, "texto")
	if err != nil {
		t.Fatal(err)
	}
	if !receiptCopy.Impresion.Copia || !strings.Contains(receiptCopy.Contenido, "COPIA") {
		t.Fatalf("receipt copy = %+v", receiptCopy)
	}
}

func TestT04_BrowserPrintRepresentations(t *testing.T) {
	svc, pool, cleanup := setupT04(t)
	defer cleanup()
	seed := seedT04(t, pool)
	ctx := context.Background()

	completion := completeByCard(t, svc, seed.EmpleadoID, []CreateSaleItem{
		{ProductoID: seed.ProductoBarraID, Cantidad: 1},
	})

	comanda, err := svc.PrintComanda(ctx, completion.Comandas[0].ID, seed.EmpleadoID, "html")
	if err != nil {
		t.Fatal(err)
	}
	if comanda.Formato != "html" || !strings.Contains(comanda.Contenido, "<!doctype html>") ||
		!strings.Contains(comanda.Contenido, "@media print") {
		t.Fatalf("comanda is not browser-printable: %+v", comanda)
	}

	receipt, err := svc.PrintReceipt(ctx, completion.Venta.ID, seed.EmpleadoID, "texto")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Formato != "texto" || !strings.Contains(receipt.Contenido, "BENGAL POS") {
		t.Fatalf("receipt text = %+v", receipt)
	}
}

func TestT04_ItemsWithoutAreaDoNotBlockCompletion(t *testing.T) {
	svc, pool, cleanup := setupT04(t)
	defer cleanup()
	seed := seedT04(t, pool)
	ctx := context.Background()

	var sinAreaID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, precio_venta_bs)
		 VALUES ('SIN-1', 'Sin área', 1000) RETURNING id`,
	).Scan(&sinAreaID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO stock_depositos (producto_id, deposito_id, cantidad) VALUES ($1, $2, 5)`,
		sinAreaID, seed.DepositoID); err != nil {
		t.Fatal(err)
	}

	completion := completeByCard(t, svc, seed.EmpleadoID, []CreateSaleItem{
		{ProductoID: seed.ProductoCocinaID, Cantidad: 1},
		{ProductoID: sinAreaID, Cantidad: 1},
	})
	if completion.Venta.Estado != "completada" {
		t.Fatalf("sale state = %s", completion.Venta.Estado)
	}
	if len(completion.Comandas) != 1 || completion.Comandas[0].AreaNombre != "Cocina" {
		t.Fatalf("comandas = %+v", completion.Comandas)
	}
	if len(completion.SinArea) != 1 || completion.SinArea[0] != "Sin área" {
		t.Fatalf("sin_area = %v", completion.SinArea)
	}
}
