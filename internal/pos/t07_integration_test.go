package pos

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupT07(t *testing.T) (*Service, *pgxpool.Pool, func()) {
	t.Helper()
	return setupT03(t)
}

func seedT07(t *testing.T, pool *pgxpool.Pool) (empleadoID, encargadoID, adminID, duenoID int64) {
	t.Helper()
	ctx := context.Background()
	rows := []struct {
		nombre string
		login  string
		rol    string
		target *int64
	}{
		{"Empleado", "emp7", "empleado", &empleadoID},
		{"Encargado", "enc7", "encargado", &encargadoID},
		{"Admin", "adm7", "administrador", &adminID},
		{"Dueño", "due7", "dueno", &duenoID},
	}
	for _, row := range rows {
		if err := pool.QueryRow(ctx,
			`INSERT INTO usuarios (nombre_completo, login, password_hash, rol)
			 VALUES ($1, $2, 'h', $3) RETURNING id`,
			row.nombre, row.login, row.rol,
		).Scan(row.target); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func mustProducto(t *testing.T, svc *Service, usuarioID int64, codigo, nombre string, stockMinimo, precio int64) *Producto {
	t.Helper()
	producto, err := svc.CreateProducto(context.Background(), CreateProductoInput{
		UsuarioID:     usuarioID,
		Codigo:        codigo,
		Nombre:        nombre,
		StockMinimo:   stockMinimo,
		PrecioVentaBs: precio,
	})
	if err != nil {
		t.Fatal(err)
	}
	return producto
}

func mustDeposito(t *testing.T, svc *Service, usuarioID int64, nombre string) *Deposito {
	t.Helper()
	deposito, err := svc.CreateDeposito(context.Background(), CreateDepositoInput{
		UsuarioID: usuarioID,
		Nombre:    nombre,
	})
	if err != nil {
		t.Fatal(err)
	}
	return deposito
}

func mustEntrada(t *testing.T, svc *Service, usuarioID, productoID, depositoID, cantidad int64) *MovimientoResultado {
	t.Helper()
	resultado, err := svc.AddMovimientoInventario(context.Background(), InventarioMovimientoInput{
		UsuarioID:  usuarioID,
		ProductoID: productoID,
		DepositoID: depositoID,
		Tipo:       "entrada",
		Cantidad:   cantidad,
		Motivo:     "carga inicial",
	})
	if err != nil {
		t.Fatal(err)
	}
	return resultado
}

func requireAppError(t *testing.T, err error, status int, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %q, got nil", message)
	}
	var appErr *AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Status != status || appErr.Message != message {
		t.Fatalf("error = (%d) %q, want (%d) %q", appErr.Status, appErr.Message, status, message)
	}
}

func stockCantidad(t *testing.T, pool *pgxpool.Pool, productoID, depositoID int64) int64 {
	t.Helper()
	var cantidad int64
	if err := pool.QueryRow(context.Background(),
		`SELECT cantidad FROM stock_depositos WHERE producto_id = $1 AND deposito_id = $2`,
		productoID, depositoID,
	).Scan(&cantidad); err != nil {
		t.Fatal(err)
	}
	return cantidad
}

func movimientoSuma(t *testing.T, pool *pgxpool.Pool, productoID, depositoID int64) int64 {
	t.Helper()
	var total int64
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(cantidad), 0) FROM movimientos_inventario
		 WHERE producto_id = $1 AND deposito_id = $2`,
		productoID, depositoID,
	).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

func movimientosCount(t *testing.T, pool *pgxpool.Pool, productoID int64) int64 {
	t.Helper()
	var total int64
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM movimientos_inventario WHERE producto_id = $1`, productoID,
	).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

// AC1: product creation, trimmed unique code, integer validation.
// AC2: edition, deactivation, no physical delete and sale visibility.
func TestT07_AC1_AC2_Productos(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	emp, _, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "  TOR-001  ", "Torta", 10, 1000)
	if producto.Codigo != "TOR-001" {
		t.Fatalf("codigo = %q, want trimmed TOR-001", producto.Codigo)
	}

	_, err := svc.CreateProducto(ctx, CreateProductoInput{
		UsuarioID: admin, Codigo: "TOR-001", Nombre: "Duplicado", PrecioVentaBs: 100,
	})
	requireAppError(t, err, 409, "El código del producto ya existe")

	for _, in := range []CreateProductoInput{
		{UsuarioID: admin, Codigo: "   ", Nombre: "X", PrecioVentaBs: 1},
		{UsuarioID: admin, Codigo: "TOR-002", Nombre: "   ", PrecioVentaBs: 1},
		{UsuarioID: admin, Codigo: "TOR-002", Nombre: "X", StockMinimo: -1, PrecioVentaBs: 1},
		{UsuarioID: admin, Codigo: "TOR-002", Nombre: "X", PrecioVentaBs: -1},
	} {
		if _, err := svc.CreateProducto(ctx, in); err == nil {
			t.Fatalf("expected validation error for %+v", in)
		}
	}
	var total int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM productos`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("products = %d, want 1 after rejected inserts", total)
	}

	otro := mustProducto(t, svc, admin, "TOR-002", "Otra", 0, 500)
	var categoriaID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO categorias (nombre) VALUES ('Postres') RETURNING id`,
	).Scan(&categoriaID); err != nil {
		t.Fatal(err)
	}
	nuevoNombre := "Torta de chocolate"
	nuevoPrecio := int64(1500)
	nuevoMinimo := int64(2)
	activo := false
	updated, err := svc.UpdateProducto(ctx, producto.ID, UpdateProductoInput{
		UsuarioID:     admin,
		Nombre:        &nuevoNombre,
		PrecioVentaBs: &nuevoPrecio,
		StockMinimo:   &nuevoMinimo,
		CategoriaID:   &categoriaID,
		Activo:        &activo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Nombre != nuevoNombre || updated.PrecioVentaBs != 1500 || updated.StockMinimo != 2 ||
		updated.Activo || updated.CategoriaID == nil || *updated.CategoriaID != categoriaID {
		t.Fatalf("unexpected updated product: %+v", updated)
	}

	duplicado := otro.Codigo
	if _, err := svc.UpdateProducto(ctx, producto.ID, UpdateProductoInput{
		UsuarioID: admin, Codigo: &duplicado,
	}); err == nil {
		t.Fatal("expected duplicate code on update")
	} else {
		requireAppError(t, err, 409, "El código del producto ya existe")
	}

	activos, err := svc.ListProductos(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(activos) != 1 || activos[0].ID != otro.ID {
		t.Fatalf("active products = %+v, want only TOR-002", activos)
	}
	todos, err := svc.ListProductos(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 2 {
		t.Fatalf("all products = %d, want 2", len(todos))
	}

	// A deactivated product cannot be added to a new sale (PR-03), but the row
	// and its historical sale detail are preserved.
	if _, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: emp,
		Items:     []CreateSaleItem{{ProductoID: producto.ID, Cantidad: 1}},
	}); err == nil || err.Error() != "El producto no está disponible" {
		t.Fatalf("sale of inactive product error = %v, want El producto no está disponible", err)
	}

	reactivar := true
	if _, err := svc.UpdateProducto(ctx, producto.ID, UpdateProductoInput{
		UsuarioID: admin, Activo: &reactivar,
	}); err != nil {
		t.Fatal(err)
	}
	activos, err = svc.ListProductos(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(activos) != 2 {
		t.Fatalf("active products after reactivation = %d, want 2", len(activos))
	}

	// A completed sale keeps its detail after the product is deactivated.
	deposito := mustDeposito(t, svc, admin, "Principal")
	mustEntrada(t, svc, admin, producto.ID, deposito.ID, 5)
	completion := completeByCard(t, svc, emp, []CreateSaleItem{{ProductoID: producto.ID, Cantidad: 1}})
	if _, err := svc.UpdateProducto(ctx, producto.ID, UpdateProductoInput{
		UsuarioID: admin, Activo: &activo,
	}); err != nil {
		t.Fatal(err)
	}
	vendida, err := svc.GetSale(ctx, completion.Venta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(vendida.Items) != 1 || vendida.Items[0].ProductoID != producto.ID ||
		vendida.Items[0].ProductoCodigo != "TOR-001" {
		t.Fatalf("completed sale detail = %+v", vendida.Items)
	}

	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM productos`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("products = %d, want no physical delete", total)
	}
}

// AC3: deposits are created, edited, deactivated and keep their stock/history.
func TestT07_AC3_Depositos(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	_, enc, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "DEP-ITEM", "Item", 0, 100)
	deposito := mustDeposito(t, svc, enc, "Principal")
	mustEntrada(t, svc, enc, producto.ID, deposito.ID, 5)

	if _, err := svc.CreateDeposito(ctx, CreateDepositoInput{UsuarioID: admin, Nombre: "Principal"}); err == nil {
		t.Fatal("expected duplicate deposit name")
	} else {
		requireAppError(t, err, 409, "El nombre del depósito ya existe")
	}

	ubicacion := "Estante 1"
	if _, err := svc.UpdateDeposito(ctx, deposito.ID, UpdateDepositoInput{
		UsuarioID: enc, Ubicacion: &ubicacion,
	}); err != nil {
		t.Fatal(err)
	}
	activo := false
	if _, err := svc.UpdateDeposito(ctx, deposito.ID, UpdateDepositoInput{
		UsuarioID: enc, Activo: &activo,
	}); err != nil {
		t.Fatal(err)
	}

	activos, err := svc.ListDepositos(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(activos) != 0 {
		t.Fatalf("active deposits = %d, want 0", len(activos))
	}
	todos, err := svc.ListDepositos(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 1 || todos[0].Ubicacion == nil || *todos[0].Ubicacion != ubicacion {
		t.Fatalf("all deposits = %+v", todos)
	}

	if _, err := svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID,
		Tipo: "salida", Cantidad: 1,
	}); err == nil {
		t.Fatal("expected inactive deposit rejection")
	} else {
		requireAppError(t, err, 422, "El depósito está inactivo")
	}
	otro := mustDeposito(t, svc, admin, "Secundario")
	if _, err := svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: producto.ID,
		DepositoOrigenID: deposito.ID, DepositoDestinoID: otro.ID, Cantidad: 1,
	}); err == nil {
		t.Fatal("expected inactive origin rejection")
	}

	stock, err := svc.GetStock(ctx, &producto.ID, &deposito.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stock) != 1 || stock[0].StockTotal != 5 {
		t.Fatalf("stock of inactive deposit = %+v, want preserved 5", stock)
	}

	var total int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM depositos`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("deposits = %d, want no physical delete", total)
	}
}

// AC4: suppliers and contact edition/deactivation.
// AC5: item-supplier associations with a single principal per item.
func TestT07_AC4_AC5_ProveedoresAsociacion(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	emp, enc, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "PROV-001", "Item", 0, 100)
	otroProducto := mustProducto(t, svc, admin, "PROV-002", "Otro", 0, 100)

	if _, err := svc.CreateProveedor(ctx, CreateProveedorInput{UsuarioID: emp, Nombre: "X"}); err == nil {
		t.Fatal("expected employee provider rejection")
	} else {
		requireAppError(t, err, 403, "No autorizado para realizar esta operación")
	}

	telefono := "70000000"
	proveedorX, err := svc.CreateProveedor(ctx, CreateProveedorInput{
		UsuarioID: admin, Nombre: "Proveedor X", Telefono: &telefono,
	})
	if err != nil {
		t.Fatal(err)
	}
	proveedorY, err := svc.CreateProveedor(ctx, CreateProveedorInput{
		UsuarioID: admin, Nombre: "Proveedor Y",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProveedor(ctx, CreateProveedorInput{UsuarioID: admin, Nombre: "   "}); err == nil {
		t.Fatal("expected required provider name")
	}

	nuevoContacto := "Ana"
	editado, err := svc.UpdateProveedor(ctx, proveedorX.ID, UpdateProveedorInput{
		UsuarioID: admin, ContactoNombre: &nuevoContacto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if editado.ContactoNombre == nil || *editado.ContactoNombre != nuevoContacto {
		t.Fatalf("provider contact not updated: %+v", editado)
	}

	if _, err := svc.AsociarProductoProveedor(ctx, producto.ID, AsociarProveedorInput{
		UsuarioID: enc, ProveedorID: proveedorX.ID,
	}); err == nil {
		t.Fatal("expected encargado association rejection")
	} else {
		requireAppError(t, err, 403, "No autorizado para realizar esta operación")
	}

	if _, err := svc.AsociarProductoProveedor(ctx, producto.ID, AsociarProveedorInput{
		UsuarioID: admin, ProveedorID: proveedorX.ID, EsPrincipal: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AsociarProductoProveedor(ctx, producto.ID, AsociarProveedorInput{
		UsuarioID: admin, ProveedorID: proveedorY.ID, EsPrincipal: true,
	}); err != nil {
		t.Fatal(err)
	}

	asociaciones, err := svc.ListProductoProveedores(ctx, producto.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(asociaciones) != 2 {
		t.Fatalf("associations = %d, want 2", len(asociaciones))
	}
	principales := 0
	for _, asociacion := range asociaciones {
		if asociacion.EsPrincipal {
			principales++
			if asociacion.ProveedorID != proveedorY.ID {
				t.Fatalf("principal = %d, want Y %d", asociacion.ProveedorID, proveedorY.ID)
			}
		}
	}
	if principales != 1 {
		t.Fatalf("principal associations = %d, want exactly 1", principales)
	}

	// Inactive suppliers keep their associations but cannot receive new ones.
	inactivo := false
	if _, err := svc.UpdateProveedor(ctx, proveedorX.ID, UpdateProveedorInput{
		UsuarioID: admin, Activo: &inactivo,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateProveedor(ctx, proveedorX.ID, UpdateProveedorInput{
		UsuarioID: admin, ContactoNombre: &nuevoContacto,
	}); err == nil {
		t.Fatal("expected inactive provider contact edit rejection")
	} else {
		requireAppError(t, err, 422, "El proveedor está inactivo")
	}
	if _, err := svc.AsociarProductoProveedor(ctx, otroProducto.ID, AsociarProveedorInput{
		UsuarioID: admin, ProveedorID: proveedorX.ID, EsPrincipal: true,
	}); err == nil {
		t.Fatal("expected inactive provider association rejection")
	} else {
		requireAppError(t, err, 422, "El proveedor está inactivo")
	}
	if len(asociaciones) != 2 {
		t.Fatal("deactivation must preserve associations")
	}

	// An inactive product cannot receive new associations (PR-03).
	if _, err := svc.UpdateProducto(ctx, otroProducto.ID, UpdateProductoInput{
		UsuarioID: admin, Activo: &inactivo,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AsociarProductoProveedor(ctx, otroProducto.ID, AsociarProveedorInput{
		UsuarioID: admin, ProveedorID: proveedorY.ID,
	}); err == nil {
		t.Fatal("expected inactive product association rejection")
	} else {
		requireAppError(t, err, 422, "El producto está inactivo")
	}

	if err := svc.RemoveProductoProveedor(ctx, producto.ID, proveedorX.ID, admin); err != nil {
		t.Fatal(err)
	}
	asociaciones, err = svc.ListProductoProveedores(ctx, producto.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(asociaciones) != 1 || asociaciones[0].ProveedorID != proveedorY.ID {
		t.Fatalf("associations after removal = %+v", asociaciones)
	}
	if err := svc.RemoveProductoProveedor(ctx, producto.ID, proveedorX.ID, admin); err == nil {
		t.Fatal("expected missing association error")
	}

	// No physical delete of suppliers.
	var total int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM proveedores`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("suppliers = %d, want no physical delete", total)
	}
}

// AC6: stock query reports every product/deposit pair and reconciles with the
// movement history. AC7: manual entradas update stock and history atomically.
func TestT07_AC6_AC7_StockEntrada(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	emp, enc, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "STK-001", "Stock", 0, 100)
	depositoA := mustDeposito(t, svc, admin, "A")
	depositoB := mustDeposito(t, svc, admin, "B")

	stock, err := svc.GetStock(ctx, &producto.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stock) != 1 || stock[0].StockTotal != 0 || len(stock[0].Depositos) != 0 {
		t.Fatalf("stock without rows = %+v, want total 0", stock)
	}

	resultado, err := svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: producto.ID, DepositoID: depositoA.ID,
		Tipo: "entrada", Cantidad: 5, Motivo: "compra",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resultado.StockResultante != 5 || resultado.Movimiento.Tipo != "entrada" ||
		resultado.Movimiento.Cantidad != 5 || resultado.Movimiento.UsuarioID != enc {
		t.Fatalf("unexpected entry result: %+v", resultado)
	}
	if got := stockCantidad(t, pool, producto.ID, depositoA.ID); got != 5 {
		t.Fatalf("stock = %d, want 5", got)
	}
	if got := movimientoSuma(t, pool, producto.ID, depositoA.ID); got != 5 {
		t.Fatalf("movement sum = %d, want 5", got)
	}

	// A transfer keeps the stock projection reconciled too.
	if _, err := svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: producto.ID,
		DepositoOrigenID: depositoA.ID, DepositoDestinoID: depositoB.ID, Cantidad: 2,
	}); err != nil {
		t.Fatal(err)
	}
	stock, err = svc.GetStock(ctx, &producto.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stock) != 1 || stock[0].StockTotal != 5 || len(stock[0].Depositos) != 2 {
		t.Fatalf("stock after transfer = %+v", stock)
	}
	for _, deposito := range stock[0].Depositos {
		if got := movimientoSuma(t, pool, producto.ID, deposito.DepositoID); got != deposito.Cantidad {
			t.Fatalf("stock %d != movement sum %d for deposit %d",
				deposito.Cantidad, got, deposito.DepositoID)
		}
	}

	for _, in := range []InventarioMovimientoInput{
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: depositoA.ID, Tipo: "entrada", Cantidad: 0},
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: depositoA.ID, Tipo: "entrada", Cantidad: -2},
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: depositoA.ID, Tipo: "otro", Cantidad: 1},
		{UsuarioID: emp, ProductoID: producto.ID, DepositoID: depositoA.ID, Tipo: "entrada", Cantidad: 1},
		{UsuarioID: enc, ProductoID: 999999, DepositoID: depositoA.ID, Tipo: "entrada", Cantidad: 1},
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: 999999, Tipo: "entrada", Cantidad: 1},
	} {
		if _, err := svc.AddMovimientoInventario(ctx, in); err == nil {
			t.Fatalf("expected movement rejection for %+v", in)
		}
	}
	if got := stockCantidad(t, pool, producto.ID, depositoA.ID); got != 3 {
		t.Fatalf("stock after rejected movements = %d, want 3", got)
	}
}

// AC8: salida with the exact ST-01 warning and confirmed negative stock.
func TestT07_AC8_SalidaStockNegativo(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	emp, enc, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "SAL-001", "Salida", 0, 100)
	deposito := mustDeposito(t, svc, admin, "Principal")
	mustEntrada(t, svc, enc, producto.ID, deposito.ID, 5)

	_, err := svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID,
		Tipo: "salida", Cantidad: 8,
	})
	requireAppError(t, err, 409,
		"La salida dejará el stock en negativo (quedará en -3). ¿Desea continuar?")
	if got := stockCantidad(t, pool, producto.ID, deposito.ID); got != 5 {
		t.Fatalf("stock after rejected exit = %d, want 5", got)
	}
	if got := movimientosCount(t, pool, producto.ID); got != 1 {
		t.Fatalf("movements after rejected exit = %d, want only the entry", got)
	}

	confirmado := mustProducto(t, svc, admin, "SAL-002", "Salida confirmada", 0, 100)
	mustEntrada(t, svc, enc, confirmado.ID, deposito.ID, 5)
	resultado, err := svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: confirmado.ID, DepositoID: deposito.ID,
		Tipo: "salida", Cantidad: 8, ConfirmarStockNegativo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resultado.StockResultante != -3 ||
		resultado.Movimiento.Cantidad != -8 ||
		!resultado.Movimiento.StockNegativoConfirmado {
		t.Fatalf("unexpected confirmed exit: %+v", resultado)
	}
	if got := stockCantidad(t, pool, confirmado.ID, deposito.ID); got != -3 {
		t.Fatalf("stock after confirmed exit = %d, want -3", got)
	}

	exacto := mustProducto(t, svc, admin, "SAL-003", "Salida exacta", 0, 100)
	mustEntrada(t, svc, enc, exacto.ID, deposito.ID, 5)
	resultado, err = svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: exacto.ID, DepositoID: deposito.ID,
		Tipo: "salida", Cantidad: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resultado.StockResultante != 0 || resultado.Movimiento.StockNegativoConfirmado {
		t.Fatalf("exact exit = %+v, want 0 without warning", resultado)
	}

	if _, err := svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: emp, ProductoID: producto.ID, DepositoID: deposito.ID,
		Tipo: "salida", Cantidad: 1,
	}); err == nil {
		t.Fatal("expected employee rejection")
	}
}

// AC9: ajuste uses sentido and requires a motive; it follows the same ST-01
// confirmation rule when it lowers stock below zero.
func TestT07_AC9_Ajuste(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	_, enc, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "AJU-001", "Ajuste", 0, 100)
	deposito := mustDeposito(t, svc, admin, "Principal")
	mustEntrada(t, svc, enc, producto.ID, deposito.ID, 10)

	sube, err := svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID,
		Tipo: "ajuste", Cantidad: 3, Sentido: "sube", Motivo: "conteo físico",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sube.StockResultante != 13 || sube.Movimiento.Cantidad != 3 || sube.Movimiento.Tipo != "ajuste" {
		t.Fatalf("unexpected positive adjustment: %+v", sube)
	}

	_, err = svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID,
		Tipo: "ajuste", Cantidad: 20, Sentido: "baja", Motivo: "merma",
	})
	requireAppError(t, err, 409,
		"La salida dejará el stock en negativo (quedará en -7). ¿Desea continuar?")
	if got := stockCantidad(t, pool, producto.ID, deposito.ID); got != 13 {
		t.Fatalf("stock after rejected adjustment = %d, want 13", got)
	}

	baja, err := svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID,
		Tipo: "ajuste", Cantidad: 20, Sentido: "baja", Motivo: "merma",
		ConfirmarStockNegativo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if baja.StockResultante != -7 || baja.Movimiento.Cantidad != -20 || !baja.Movimiento.StockNegativoConfirmado {
		t.Fatalf("unexpected negative adjustment: %+v", baja)
	}

	for _, in := range []InventarioMovimientoInput{
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID, Tipo: "ajuste", Cantidad: 0, Sentido: "sube", Motivo: "x"},
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID, Tipo: "ajuste", Cantidad: 1, Sentido: "otro", Motivo: "x"},
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID, Tipo: "ajuste", Cantidad: 1, Sentido: "sube", Motivo: "   "},
		{UsuarioID: enc, ProductoID: producto.ID, DepositoID: deposito.ID, Tipo: "ajuste", Cantidad: 1, Motivo: "x"},
	} {
		if _, err := svc.AddMovimientoInventario(ctx, in); err == nil {
			t.Fatalf("expected adjustment rejection for %+v", in)
		}
	}
}

// AC10: atomic transfers with the same group id, two rows and ST-01 confirm.
func TestT07_AC10_Traslados(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	_, enc, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "TRA-001", "Traslado", 0, 100)
	depositoA := mustDeposito(t, svc, admin, "A")
	depositoB := mustDeposito(t, svc, admin, "B")
	mustEntrada(t, svc, enc, producto.ID, depositoA.ID, 10)

	resultado, err := svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: producto.ID,
		DepositoOrigenID: depositoA.ID, DepositoDestinoID: depositoB.ID,
		Cantidad: 4, Motivo: "reubicación",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resultado.StockOrigen != 6 || resultado.StockDestino != 4 {
		t.Fatalf("transfer stocks = %d/%d, want 6/4", resultado.StockOrigen, resultado.StockDestino)
	}
	if resultado.GrupoID == "" || resultado.Origen.GrupoID != resultado.GrupoID ||
		resultado.Destino.GrupoID != resultado.GrupoID {
		t.Fatalf("transfer group mismatch: %+v", resultado)
	}
	if resultado.Origen.Tipo != "traslado" || resultado.Destino.Tipo != "traslado" ||
		resultado.Origen.Cantidad != -4 || resultado.Destino.Cantidad != 4 {
		t.Fatalf("unexpected transfer rows: %+v", resultado)
	}
	if resultado.Origen.UsuarioID != enc || resultado.Destino.UsuarioID != enc {
		t.Fatal("transfer rows must keep the same user")
	}
	if !resultado.Origen.CreadoEn.Equal(resultado.Destino.CreadoEn) {
		t.Fatalf("transfer dates differ: %v vs %v", resultado.Origen.CreadoEn, resultado.Destino.CreadoEn)
	}
	if got := stockCantidad(t, pool, producto.ID, depositoA.ID); got != 6 {
		t.Fatalf("origin stock = %d, want 6", got)
	}
	if got := stockCantidad(t, pool, producto.ID, depositoB.ID); got != 4 {
		t.Fatalf("destination stock = %d, want 4", got)
	}

	// Invalid transfers persist nothing.
	before := movimientosCount(t, pool, producto.ID)
	if _, err := svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: producto.ID,
		DepositoOrigenID: depositoA.ID, DepositoDestinoID: depositoA.ID, Cantidad: 1,
	}); err == nil {
		t.Fatal("expected same-deposit rejection")
	}
	if _, err := svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: producto.ID,
		DepositoOrigenID: depositoA.ID, DepositoDestinoID: depositoB.ID, Cantidad: 0,
	}); err == nil {
		t.Fatal("expected zero quantity rejection")
	}
	_, err = svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: producto.ID,
		DepositoOrigenID: depositoB.ID, DepositoDestinoID: depositoA.ID, Cantidad: 100,
	})
	requireAppError(t, err, 409,
		"La salida dejará el stock en negativo (quedará en -96). ¿Desea continuar?")
	if got := movimientosCount(t, pool, producto.ID); got != before {
		t.Fatalf("movements after rejected transfer = %d, want %d", got, before)
	}

	confirmado, err := svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: producto.ID,
		DepositoOrigenID: depositoB.ID, DepositoDestinoID: depositoA.ID,
		Cantidad: 100, ConfirmarStockNegativo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmado.StockOrigen != -96 || confirmado.StockDestino != 106 {
		t.Fatalf("confirmed transfer stocks = %d/%d, want -96/106",
			confirmado.StockOrigen, confirmado.StockDestino)
	}
	if !confirmado.Origen.StockNegativoConfirmado || confirmado.Destino.StockNegativoConfirmado {
		t.Fatalf("negative flag on wrong transfer half: %+v", confirmado)
	}
	if got := movimientosCount(t, pool, producto.ID); got != before+2 {
		t.Fatalf("movements after confirmed transfer = %d, want %d", got, before+2)
	}
}

// AC11: low-stock report grouped by principal supplier, including inactive
// suppliers and the "Sin proveedor" group.
func TestT07_AC11_BajoStock(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	_, _, admin, _ := seedT07(t, pool)

	deposito := mustDeposito(t, svc, admin, "Principal")

	proveedorX, err := svc.CreateProveedor(ctx, CreateProveedorInput{UsuarioID: admin, Nombre: "X"})
	if err != nil {
		t.Fatal(err)
	}
	proveedorY, err := svc.CreateProveedor(ctx, CreateProveedorInput{UsuarioID: admin, Nombre: "Y"})
	if err != nil {
		t.Fatal(err)
	}

	productoA := mustProducto(t, svc, admin, "BAJO-A", "A", 10, 100)
	productoB := mustProducto(t, svc, admin, "BAJO-B", "B", 5, 100)
	productoSin := mustProducto(t, svc, admin, "BAJO-S", "Sin proveedor", 1, 100)
	productoAlto := mustProducto(t, svc, admin, "BAJO-OK", "Suficiente", 2, 100)
	productoInactivo := mustProducto(t, svc, admin, "BAJO-X", "Inactivo", 100, 100)

	for _, item := range []struct {
		producto *Producto
		cantidad int64
	}{
		{productoA, 10},
		{productoB, 2},
		{productoAlto, 5},
	} {
		mustEntrada(t, svc, admin, item.producto.ID, deposito.ID, item.cantidad)
	}

	for _, asociacion := range []struct {
		producto  *Producto
		proveedor *Proveedor
		principal bool
	}{
		{productoA, proveedorX, true},
		{productoB, proveedorY, true},
		{productoAlto, proveedorX, true},
	} {
		if _, err := svc.AsociarProductoProveedor(ctx, asociacion.producto.ID, AsociarProveedorInput{
			UsuarioID: admin, ProveedorID: asociacion.proveedor.ID, EsPrincipal: asociacion.principal,
		}); err != nil {
			t.Fatal(err)
		}
	}

	inactivo := false
	if _, err := svc.UpdateProveedor(ctx, proveedorX.ID, UpdateProveedorInput{
		UsuarioID: admin, Activo: &inactivo,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateProducto(ctx, productoInactivo.ID, UpdateProductoInput{
		UsuarioID: admin, Activo: &inactivo,
	}); err != nil {
		t.Fatal(err)
	}

	reporte, err := svc.BajoStock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(reporte.Grupos) != 3 {
		t.Fatalf("groups = %+v, want 3 (X, Y, Sin proveedor)", reporte.Grupos)
	}

	findGroup := func(name string) *BajoStockGrupo {
		for i := range reporte.Grupos {
			if reporte.Grupos[i].ProveedorNombre == name {
				return &reporte.Grupos[i]
			}
		}
		return nil
	}
	grupoX := findGroup("X")
	if grupoX == nil || grupoX.ProveedorActivo == nil || *grupoX.ProveedorActivo {
		t.Fatalf("group X missing or not reported as inactive: %+v", grupoX)
	}
	if len(grupoX.Productos) != 1 || grupoX.Productos[0].ProductoID != productoA.ID {
		t.Fatalf("group X products = %+v, want only A at the minimum", grupoX.Productos)
	}
	if grupoX.Productos[0].StockTotal != 10 || grupoX.Productos[0].StockMinimo != 10 {
		t.Fatalf("product A stock = %+v, want 10 <= 10", grupoX.Productos[0])
	}
	if len(grupoX.Productos[0].Depositos) != 1 ||
		grupoX.Productos[0].Depositos[0].Cantidad != 10 {
		t.Fatalf("product A breakdown = %+v", grupoX.Productos[0].Depositos)
	}

	grupoY := findGroup("Y")
	if grupoY == nil || len(grupoY.Productos) != 1 || grupoY.Productos[0].ProductoID != productoB.ID {
		t.Fatalf("group Y = %+v", grupoY)
	}
	grupoSin := findGroup("Sin proveedor")
	if grupoSin == nil || grupoSin.ProveedorID != nil ||
		len(grupoSin.Productos) != 1 || grupoSin.Productos[0].ProductoID != productoSin.ID {
		t.Fatalf("sin proveedor group = %+v", grupoSin)
	}
	for _, grupo := range reporte.Grupos {
		for _, producto := range grupo.Productos {
			if producto.ProductoID == productoAlto.ID || producto.ProductoID == productoInactivo.ID {
				t.Fatalf("unexpected product in low-stock report: %+v", producto)
			}
		}
	}
}

// AC12: inventory and catalog history is insert-only and never deleted.
func TestT07_AC12_HistorialInmutable(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	_, enc, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "HIS-001", "Historial", 0, 100)
	deposito := mustDeposito(t, svc, admin, "Principal")
	resultado := mustEntrada(t, svc, enc, producto.ID, deposito.ID, 5)

	if _, err := pool.Exec(ctx,
		`UPDATE movimientos_inventario SET motivo = 'alterado' WHERE id = $1`,
		resultado.Movimiento.ID,
	); err == nil || !strings.Contains(err.Error(), "solo inserción") {
		t.Fatalf("UPDATE history error = %v, want insert-only trigger", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM movimientos_inventario WHERE id = $1`,
		resultado.Movimiento.ID,
	); err == nil || !strings.Contains(err.Error(), "solo inserción") {
		t.Fatalf("DELETE history error = %v, want insert-only trigger", err)
	}

	nombre := "Renombrado"
	if _, err := svc.UpdateProducto(ctx, producto.ID, UpdateProductoInput{
		UsuarioID: admin, Nombre: &nombre,
	}); err != nil {
		t.Fatal(err)
	}
	if got := stockCantidad(t, pool, producto.ID, deposito.ID); got != 5 {
		t.Fatalf("stock after catalog edit = %d, want unchanged 5", got)
	}
	if got := movimientosCount(t, pool, producto.ID); got != 1 {
		t.Fatalf("history rows = %d, want 1", got)
	}
}

// AC13: role boundaries for catalog, suppliers and inventory mutations.
func TestT07_AC13_Autorizacion(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	emp, enc, admin, dueno := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "ROL-001", "Roles", 0, 100)
	deposito := mustDeposito(t, svc, admin, "Principal")
	depositoB := mustDeposito(t, svc, admin, "Secundario")

	noAutorizado := func(err error) {
		t.Helper()
		requireAppError(t, err, 403, "No autorizado para realizar esta operación")
	}

	_, err := svc.CreateProducto(ctx, CreateProductoInput{
		UsuarioID: emp, Codigo: "EMP-001", Nombre: "No", PrecioVentaBs: 1,
	})
	noAutorizado(err)
	_, err = svc.CreateDeposito(ctx, CreateDepositoInput{UsuarioID: emp, Nombre: "No"})
	noAutorizado(err)
	_, err = svc.CreateProveedor(ctx, CreateProveedorInput{UsuarioID: emp, Nombre: "No"})
	noAutorizado(err)
	_, err = svc.AddMovimientoInventario(ctx, InventarioMovimientoInput{
		UsuarioID: emp, ProductoID: producto.ID, DepositoID: deposito.ID,
		Tipo: "entrada", Cantidad: 1,
	})
	noAutorizado(err)
	_, err = svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: emp, ProductoID: producto.ID,
		DepositoOrigenID: deposito.ID, DepositoDestinoID: depositoB.ID, Cantidad: 1,
	})
	noAutorizado(err)

	var total int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM productos`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("products after employee attempts = %d, want only the admin product", total)
	}

	// Encargado manages products, deposits, movements and transfers.
	productoEnc := mustProducto(t, svc, enc, "ENC-001", "Encargado", 0, 100)
	depositoEnc := mustDeposito(t, svc, enc, "Encargado")
	mustEntrada(t, svc, enc, productoEnc.ID, depositoEnc.ID, 3)
	if _, err := svc.TransferirStock(ctx, TrasladoInput{
		UsuarioID: enc, ProductoID: productoEnc.ID,
		DepositoOrigenID: depositoEnc.ID, DepositoDestinoID: depositoB.ID, Cantidad: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProveedor(ctx, CreateProveedorInput{UsuarioID: enc, Nombre: "No"}); err == nil {
		t.Fatal("expected encargado provider rejection")
	} else {
		noAutorizado(err)
	}

	// Only admin and dueño manage suppliers.
	for _, usuarioID := range []int64{admin, dueno} {
		if _, err := svc.CreateProveedor(ctx, CreateProveedorInput{
			UsuarioID: usuarioID, Nombre: "Proveedor autorizado",
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// AC9/AC7 unit validation without a database.
func TestT07_NormalizeMovimiento(t *testing.T) {
	tipo, cantidad, motivo, err := normalizeMovimiento(" SALIDA ", 4, "", "  ")
	if err != nil || tipo != "salida" || cantidad != -4 || motivo != nil {
		t.Fatalf("salida normalization = %q %d %v %v", tipo, cantidad, motivo, err)
	}
	tipo, cantidad, motivo, err = normalizeMovimiento("ajuste", 3, "SUBE", " conteo ")
	if err != nil || tipo != "ajuste" || cantidad != 3 || motivo == nil || *motivo != "conteo" {
		t.Fatalf("ajuste sube normalization = %q %d %v %v", tipo, cantidad, motivo, err)
	}
	tipo, cantidad, _, err = normalizeMovimiento("ajuste", 3, "baja", "merma")
	if err != nil || cantidad != -3 {
		t.Fatalf("ajuste baja normalization = %q %d %v", tipo, cantidad, err)
	}
	for _, tc := range []struct {
		tipo     string
		cantidad int64
		sentido  string
		motivo   string
	}{
		{"entrada", 0, "", ""},
		{"entrada", -1, "", ""},
		{"otro", 1, "", ""},
		{"ajuste", 1, "", "x"},
		{"ajuste", 1, "otro", "x"},
		{"ajuste", 1, "sube", " "},
	} {
		if _, _, _, err := normalizeMovimiento(tc.tipo, tc.cantidad, tc.sentido, tc.motivo); err == nil {
			t.Fatalf("expected normalization error for %+v", tc)
		}
	}
}
