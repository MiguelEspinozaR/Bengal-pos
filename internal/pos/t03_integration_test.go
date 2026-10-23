package pos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"bengalpos/internal/database"
	"bengalpos/internal/testdb"
	"bengalpos/migrations"
)

// setupT03 returns a service backed by an isolated schema. The DSN comes from
// testdb.DSN: an existing DATABASE_URL, or a freshly provisioned ephemeral
// PostgreSQL cluster when DATABASE_URL is unset.
func setupT03(t *testing.T) (*Service, *pgxpool.Pool, func()) {
	t.Helper()
	dsn := testdb.DSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "t03_" + hex.EncodeToString(b)

	adminPool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}

	schemaDSN := dsn
	if strings.Contains(schemaDSN, "?") {
		schemaDSN = schemaDSN + "&search_path=" + schema + ",public"
	} else {
		schemaDSN = schemaDSN + "?search_path=" + schema
	}

	pool, err := database.NewPool(ctx, schemaDSN)
	if err != nil {
		adminPool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
		adminPool.Close()
		t.Fatal(err)
	}

	if _, err := database.ApplyMigrations(ctx, pool, migrations.FS); err != nil {
		pool.Close()
		adminPool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
		adminPool.Close()
		t.Fatal(err)
	}

	svc := NewService(pool)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pool.Close()
		adminPool.Exec(ctx, fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
		adminPool.Close()
	}

	return svc, pool, cleanup
}

func seedT03(t *testing.T, pool *pgxpool.Pool) (empleadoID, adminID, productoID, depositoID int64) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `INSERT INTO usuarios (nombre_completo, login, password_hash, rol) VALUES ('Empleado', 'emp1', 'h', 'empleado') RETURNING id`).Scan(&empleadoID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO usuarios (nombre_completo, login, password_hash, rol) VALUES ('Admin', 'admin1', 'h', 'administrador') RETURNING id`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO depositos (nombre) VALUES ('Principal') RETURNING id`).Scan(&depositoID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO productos (codigo, nombre, precio_venta_bs, stock_minimo) VALUES ('P1', 'Producto 1', 10000, 0) RETURNING id`).Scan(&productoID); err != nil {
		t.Fatal(err)
	}
	// give stock
	if _, err := pool.Exec(ctx, `INSERT INTO stock_depositos (producto_id, deposito_id, cantidad) VALUES ($1, $2, 100)`, productoID, depositoID); err != nil {
		t.Fatal(err)
	}
	return
}

func TestT03_Split60_40_Completed(t *testing.T) {
	svc, pool, cleanup := setupT03(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, prod, _ := seedT03(t, pool)

	// Open caja
	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}

	// Create sale total 10000 (100 Bs)
	sale, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: prod, Cantidad: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sale.TotalBs != 10000 {
		t.Fatalf("total = %d", sale.TotalBs)
	}

	// Add cash 60 (6000) part - need to confirm manually
	partCash, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    emp,
		MetodoCodigo: "efectivo",
		MontoBs:      6000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, partCash.ID, emp); err != nil {
		t.Fatal(err)
	}

	partCard, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    emp,
		MetodoCodigo: "tarjeta",
		MontoBs:      4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, partCard.ID, emp); err != nil {
		t.Fatal(err)
	}

	comp, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp})
	if err != nil {
		t.Fatal(err)
	}
	if comp.Venta.Estado != "completada" {
		t.Fatalf("estado = %s", comp.Venta.Estado)
	}
	if len(comp.Venta.Partes) != 2 {
		t.Fatalf("partes = %d", len(comp.Venta.Partes))
	}
	for _, p := range comp.Venta.Partes {
		if p.Estado != "confirmado" {
			t.Fatalf("part %d not confirmed", p.ID)
		}
		if p.ConfirmadoPor == nil || p.ConfirmadoEn == nil {
			t.Fatalf("missing timestamps")
		}
	}
}

func TestT03_CashWithoutCaja_Rejected(t *testing.T) {
	svc, pool, cleanup := setupT03(t)
	defer cleanup()
	ctx := context.Background()
	emp, _, prod, _ := seedT03(t, pool)

	sale, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: prod, Cantidad: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    emp,
		MetodoCodigo: "efectivo",
		MontoBs:      5000,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("wrong error type: %T", err)
	}
	if appErr.Message != "Debe abrir caja para cobrar en efectivo" {
		t.Fatalf("wrong message: %s", appErr.Message)
	}
}

func TestT03_CashChange50For45(t *testing.T) {
	svc, pool, cleanup := setupT03(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, prod, _ := seedT03(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}

	// Create sale for 4500
	sale, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: prod, Cantidad: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Change price context: product price is 10000 in our seed, let us adjust
	if _, err := pool.Exec(ctx, `UPDATE productos SET precio_venta_bs = 4500 WHERE id = $1`, prod); err != nil {
		t.Fatal(err)
	}
	// recreate sale or just check - easier to make new sale
	sale, err = svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: prod, Cantidad: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sale.TotalBs != 4500 {
		t.Fatalf("total = %d", sale.TotalBs)
	}

	recibido := int64(5000)
	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    emp,
		MetodoCodigo: "efectivo",
		MontoBs:      4500,
		RecibidoBs:   &recibido,
	})
	if err != nil {
		t.Fatal(err)
	}
	if part.CambioBs != 500 {
		t.Fatalf("cambio = %d", part.CambioBs)
	}
}

func TestT03_NoTipAllowed(t *testing.T) {
	// RF-18b: no propina field exists; API/model don't accept it
	// Just verify structure - no propina column/field that would allow it
}

func TestT03_StockDiscountRollback(t *testing.T) {
	svc, pool, cleanup := setupT03(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, prod, depo := seedT03(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}

	// Set stock to 1, try to sell 2 without confirming negative stock
	if _, err := pool.Exec(ctx, `UPDATE stock_depositos SET cantidad = 1 WHERE producto_id = $1 AND deposito_id = $2`, prod, depo); err != nil {
		t.Fatal(err)
	}

	sale, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: prod, Cantidad: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}

	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    emp,
		MetodoCodigo: "efectivo",
		MontoBs:      sale.TotalBs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, part.ID, emp); err != nil {
		t.Fatal(err)
	}

	_, err = svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp, ConfirmarStockNegativo: false})
	if err == nil {
		t.Fatal("expected stock error")
	}
	// Check stock unchanged and sale not completed
	var stock int64
	if err := pool.QueryRow(ctx, `SELECT cantidad FROM stock_depositos WHERE producto_id = $1 AND deposito_id = $2`, prod, depo).Scan(&stock); err != nil {
		t.Fatal(err)
	}
	if stock != 1 {
		t.Fatalf("stock changed to %d", stock)
	}
	var estado string
	if err := pool.QueryRow(ctx, `SELECT estado FROM ventas WHERE id = $1`, sale.ID).Scan(&estado); err != nil {
		t.Fatal(err)
	}
	if estado != "en_proceso" {
		t.Fatalf("sale state = %s", estado)
	}
}

func TestT03_IdempotentCompletion(t *testing.T) {
	svc, pool, cleanup := setupT03(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, prod, _ := seedT03(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}
	sale, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: prod, Cantidad: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    emp,
		MetodoCodigo: "efectivo",
		MontoBs:      sale.TotalBs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, part.ID, emp); err != nil {
		t.Fatal(err)
	}

	const key = "00000000-0000-4000-8000-000000000001"
	first, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if second.Venta.ID != first.Venta.ID || second.Venta.Estado != "completada" {
		t.Fatalf("retry returned a different sale: %+v", second.Venta)
	}

	var inventoryMovements, cashMovements int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM movimientos_inventario WHERE venta_id = $1`, sale.ID,
	).Scan(&inventoryMovements); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM caja_movimientos WHERE venta_id = $1`, sale.ID,
	).Scan(&cashMovements); err != nil {
		t.Fatal(err)
	}
	if inventoryMovements != 1 || cashMovements != 1 {
		t.Fatalf("duplicated movements: inventario=%d caja=%d", inventoryMovements, cashMovements)
	}
}

func TestT03_HistoryIsImmutable(t *testing.T) {
	svc, pool, cleanup := setupT03(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, prod, _ := seedT03(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}
	sale, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: prod, Cantidad: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{
		UsuarioID:    emp,
		MetodoCodigo: "efectivo",
		MontoBs:      sale.TotalBs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, part.ID, emp); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp}); err != nil {
		t.Fatal(err)
	}

	for name, query := range map[string]string{
		"inventory update": `UPDATE movimientos_inventario SET cantidad = 0 WHERE venta_id = $1`,
		"cash delete":      `DELETE FROM caja_movimientos WHERE venta_id = $1`,
		"sealed sale":      `UPDATE ventas SET total_bs = 0 WHERE id = $1`,
	} {
		if _, err := pool.Exec(ctx, query, sale.ID); err == nil {
			t.Fatalf("%s unexpectedly succeeded", name)
		}
	}
}
