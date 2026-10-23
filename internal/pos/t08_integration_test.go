package pos

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func mustCostoT08(t *testing.T, svc *Service, usuarioID, productoID, costoBs int64,
	fecha string, proveedorID *int64) *CostoCompra {
	t.Helper()
	costo, err := svc.RegistrarCostoCompra(context.Background(), productoID, CostoCompraInput{
		UsuarioID:   usuarioID,
		ProveedorID: proveedorID,
		CostoBs:     costoBs,
		Fecha:       fecha,
	})
	if err != nil {
		t.Fatal(err)
	}
	return costo
}

func mustRecetaT08(t *testing.T, svc *Service, usuarioID, productoID int64,
	rendimiento *int64, items []RecetaItemInput) *Receta {
	t.Helper()
	receta, err := svc.UpdateReceta(context.Background(), productoID, UpdateRecetaInput{
		UsuarioID:   usuarioID,
		Rendimiento: rendimiento,
		Items:       items,
	})
	if err != nil {
		t.Fatal(err)
	}
	return receta
}

func mustCostoMargenT08(t *testing.T, svc *Service, productoID int64) *CostoMargen {
	t.Helper()
	resultado, err := svc.CostoMargen(context.Background(), productoID)
	if err != nil {
		t.Fatal(err)
	}
	return resultado
}

func int64PtrT08(value int64) *int64 { return &value }

func countRowsT08(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var total int64
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

func requireRecetaItemsT08(t *testing.T, receta *Receta, expected map[int64]int64) {
	t.Helper()
	if len(receta.Items) != len(expected) {
		t.Fatalf("recipe items = %+v, want %d ingredients", receta.Items, len(expected))
	}
	for _, item := range receta.Items {
		cantidad, ok := expected[item.IngredienteID]
		if !ok || item.Cantidad != cantidad {
			t.Fatalf("recipe item = %+v, want ingredient %d with %d", item, item.IngredienteID, cantidad)
		}
	}
}

// AC1: purchase-cost registration, validation, read-only history and trigger
// immutability (RF-27/RT-04).
func TestT08_AC1_CostosHistorialInmutable(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	ctx := context.Background()
	emp, _, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "COST-001", "Harina", 0, 1000)
	proveedor, err := svc.CreateProveedor(ctx, CreateProveedorInput{
		UsuarioID: admin, Nombre: "Proveedor costo",
	})
	if err != nil {
		t.Fatal(err)
	}
	proveedorID := proveedor.ID
	costo := mustCostoT08(t, svc, admin, producto.ID, 700, "2026-10-01", &proveedorID)
	if costo.CostoBs != 700 || costo.Fecha != "2026-10-01" ||
		costo.ProductoID != producto.ID || costo.UsuarioID != admin ||
		costo.ProveedorID == nil || *costo.ProveedorID != proveedor.ID ||
		costo.ProveedorNombre != proveedor.Nombre {
		t.Fatalf("unexpected cost row: %+v", costo)
	}

	casos := []struct {
		name string
		in   CostoCompraInput
		err  *AppError
	}{
		{"costo negativo", CostoCompraInput{
			UsuarioID: admin, CostoBs: -1, Fecha: "2026-10-01",
		}, ErrCostoInvalido},
		{"fecha invalida", CostoCompraInput{
			UsuarioID: admin, CostoBs: 700, Fecha: "2026-13-40",
		}, ErrCostoFechaInvalida},
		{"fecha sin formato", CostoCompraInput{
			UsuarioID: admin, CostoBs: 700, Fecha: "01/10/2026",
		}, ErrCostoFechaInvalida},
		{"empleado", CostoCompraInput{
			UsuarioID: emp, CostoBs: 700, Fecha: "2026-10-01",
		}, ErrNoAutorizado},
	}
	for _, tc := range casos {
		if _, err := svc.RegistrarCostoCompra(ctx, producto.ID, tc.in); err == nil {
			t.Fatalf("%s: expected rejection", tc.name)
		} else {
			requireAppError(t, err, tc.err.Status, tc.err.Message)
		}
	}
	if _, err := svc.RegistrarCostoCompra(ctx, 999999, CostoCompraInput{
		UsuarioID: admin, CostoBs: 700, Fecha: "2026-10-01",
	}); err == nil {
		t.Fatal("expected missing product rejection")
	} else {
		requireAppError(t, err, ErrProductoNoExiste.Status, ErrProductoNoExiste.Message)
	}

	var total int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM costos_compra`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("cost rows after rejected inputs = %d, want 1", total)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE costos_compra SET costo_bs = 1 WHERE id = $1`, costo.ID,
	); err == nil || !strings.Contains(err.Error(), "solo inserción") {
		t.Fatalf("UPDATE cost error = %v, want insert-only trigger", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM costos_compra WHERE id = $1`, costo.ID,
	); err == nil || !strings.Contains(err.Error(), "solo inserción") {
		t.Fatalf("DELETE cost error = %v, want insert-only trigger", err)
	}

	history, err := svc.ListCostosProducto(ctx, producto.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != costo.ID || history[0].CostoBs != 700 {
		t.Fatalf("cost history = %+v, want the registered row", history)
	}
	if _, err := svc.ListCostosProducto(ctx, 999999); err == nil {
		t.Fatal("expected missing product on history listing")
	}
}

// AC2: latest cost by date/creado_en; a product without costs reports nil.
func TestT08_AC2_CostoVigente(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	_, _, admin, _ := seedT07(t, pool)

	vacio := mustProducto(t, svc, admin, "VIG-000", "Sin costo", 0, 1000)
	resultado := mustCostoMargenT08(t, svc, vacio.ID)
	if resultado.CostoCompleto || resultado.CostoBs != nil || resultado.MargenBs != nil {
		t.Fatalf("product without cost = %+v, want nil cost/margin", resultado)
	}
	if resultado.Origen != "costo_compra" || len(resultado.IngredientesSinCosto) != 0 {
		t.Fatalf("product without cost metadata = %+v", resultado)
	}

	producto := mustProducto(t, svc, admin, "VIG-001", "Costo vigente", 0, 1000)
	mustCostoT08(t, svc, admin, producto.ID, 700, "2026-10-01", nil)
	mustCostoT08(t, svc, admin, producto.ID, 750, "2026-10-05", nil)
	if got := mustCostoMargenT08(t, svc, producto.ID); got.CostoBs == nil || *got.CostoBs != 750 {
		t.Fatalf("current cost = %+v, want 750", got)
	}

	// Same date: the later insert (greater creado_en) wins.
	mismoDia := mustProducto(t, svc, admin, "VIG-002", "Mismo día", 0, 1000)
	mustCostoT08(t, svc, admin, mismoDia.ID, 700, "2026-10-10", nil)
	time.Sleep(2 * time.Millisecond)
	mustCostoT08(t, svc, admin, mismoDia.ID, 750, "2026-10-10", nil)
	if got := mustCostoMargenT08(t, svc, mismoDia.ID); got.CostoBs == nil || *got.CostoBs != 750 {
		t.Fatalf("same-date current cost = %+v, want 750", got)
	}
}

// AC3: recipe replacement is validated and atomic; ingredients are active,
// unique and different from the elaborated product.
func TestT08_AC3_Receta(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	_, _, admin, _ := seedT07(t, pool)
	ctx := context.Background()

	producto := mustProducto(t, svc, admin, "REC-A", "Producto A", 0, 1000)
	ingB := mustProducto(t, svc, admin, "REC-B", "Ingrediente B", 0, 0)
	ingC := mustProducto(t, svc, admin, "REC-C", "Ingrediente C", 0, 0)
	ingD := mustProducto(t, svc, admin, "REC-D", "Ingrediente D", 0, 0)

	receta := mustRecetaT08(t, svc, admin, producto.ID, int64PtrT08(2), []RecetaItemInput{
		{IngredienteID: ingB.ID, Cantidad: 2},
		{IngredienteID: ingC.ID, Cantidad: 1},
	})
	if receta.Rendimiento != 2 || len(receta.Items) != 2 {
		t.Fatalf("created recipe = %+v", receta)
	}
	loaded, err := svc.GetReceta(ctx, producto.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireRecetaItemsT08(t, loaded, map[int64]int64{ingB.ID: 2, ingC.ID: 1})

	inactivo := false
	if _, err := svc.UpdateProducto(ctx, ingD.ID, UpdateProductoInput{
		UsuarioID: admin, Activo: &inactivo,
	}); err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		name  string
		items []RecetaItemInput
		err   *AppError
	}{
		{"cantidad cero", []RecetaItemInput{{IngredienteID: ingB.ID, Cantidad: 0}}, ErrRecetaCantidad},
		{"cantidad negativa", []RecetaItemInput{{IngredienteID: ingB.ID, Cantidad: -1}}, ErrRecetaCantidad},
		{"ingrediente duplicado", []RecetaItemInput{
			{IngredienteID: ingB.ID, Cantidad: 1},
			{IngredienteID: ingB.ID, Cantidad: 2},
		}, ErrRecetaIngredienteRepetido},
		{"autorreferencia", []RecetaItemInput{{IngredienteID: producto.ID, Cantidad: 1}}, ErrRecetaAutoingrediente},
		{"ingrediente inexistente", []RecetaItemInput{{IngredienteID: 999999, Cantidad: 1}}, ErrIngredienteNoExiste},
		{"ingrediente inactivo", []RecetaItemInput{{IngredienteID: ingD.ID, Cantidad: 1}}, ErrIngredienteInactivo},
	}
	for _, tc := range casos {
		if _, err := svc.UpdateReceta(ctx, producto.ID, UpdateRecetaInput{
			UsuarioID: admin, Items: tc.items,
		}); err == nil {
			t.Fatalf("%s: expected rejection", tc.name)
		} else {
			requireAppError(t, err, tc.err.Status, tc.err.Message)
		}
		actual, err := svc.GetReceta(ctx, producto.ID)
		if err != nil {
			t.Fatal(err)
		}
		requireRecetaItemsT08(t, actual, map[int64]int64{ingB.ID: 2, ingC.ID: 1})
	}

	// The list is required: omitting it must not silently delete the recipe.
	if _, err := svc.UpdateReceta(ctx, producto.ID, UpdateRecetaInput{
		UsuarioID: admin,
	}); err == nil {
		t.Fatal("expected missing item-list rejection")
	} else {
		requireAppError(t, err, ErrRecetaItemsRequeridos.Status, ErrRecetaItemsRequeridos.Message)
	}

	// Empty replacement removes the recipe without deleting the product.
	if _, err := svc.UpdateReceta(ctx, producto.ID, UpdateRecetaInput{
		UsuarioID: admin, Items: []RecetaItemInput{},
	}); err != nil {
		t.Fatal(err)
	}
	vacia, err := svc.GetReceta(ctx, producto.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(vacia.Items) != 0 || vacia.Rendimiento != 1 {
		t.Fatalf("empty recipe = %+v", vacia)
	}
	if got := countRowsT08(t, pool,
		`SELECT COUNT(*) FROM receta_detalles d JOIN recetas r ON r.id = d.receta_id
		 WHERE r.producto_elaborado_id = $1`, producto.ID); got != 0 {
		t.Fatalf("recipe rows after removal = %d, want 0", got)
	}

	// The database also refuses a direct self-reference.
	if _, err := pool.Exec(ctx,
		`INSERT INTO recetas (producto_elaborado_id, usuario_id) VALUES ($1, $2) RETURNING id`,
		producto.ID, admin,
	); err != nil {
		t.Fatal(err)
	}
	var recetaID int64
	if err := pool.QueryRow(ctx,
		`SELECT id FROM recetas WHERE producto_elaborado_id = $1`, producto.ID,
	).Scan(&recetaID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO receta_detalles (receta_id, producto_insumo_id, cantidad)
		 VALUES ($1, $2, 1)`, recetaID, producto.ID,
	); err == nil || !strings.Contains(err.Error(), "su propia receta") {
		t.Fatalf("direct self-reference error = %v, want database rejection", err)
	}
}

// AC4: simple product cost and margin, including a negative margin.
func TestT08_AC4_CostoMargenSimple(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	_, _, admin, _ := seedT07(t, pool)

	producto := mustProducto(t, svc, admin, "MAR-001", "Margen", 0, 1000)
	mustCostoT08(t, svc, admin, producto.ID, 700, "2026-10-01", nil)
	resultado := mustCostoMargenT08(t, svc, producto.ID)
	if resultado.CostoBs == nil || *resultado.CostoBs != 700 ||
		resultado.MargenBs == nil || *resultado.MargenBs != 300 ||
		resultado.Origen != "costo_compra" || !resultado.CostoCompleto {
		t.Fatalf("positive margin = %+v, want cost 700 margin 300", resultado)
	}

	negativo := mustProducto(t, svc, admin, "MAR-002", "Margen negativo", 0, 500)
	mustCostoT08(t, svc, admin, negativo.ID, 700, "2026-10-01", nil)
	resultado = mustCostoMargenT08(t, svc, negativo.ID)
	if resultado.MargenBs == nil || *resultado.MargenBs != -200 {
		t.Fatalf("negative margin = %+v, want -200", resultado)
	}
}

// AC5: elaborated cost is the integer sum of ingredient costs, resolved
// recursively; a recipe takes precedence over the product's own cost.
func TestT08_AC5_CostoElaboradoRecursivo(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	_, _, admin, _ := seedT07(t, pool)

	ingX := mustProducto(t, svc, admin, "ELAB-X", "Insumo X", 0, 0)
	ingY := mustProducto(t, svc, admin, "ELAB-Y", "Insumo Y", 0, 0)
	ingC := mustProducto(t, svc, admin, "ELAB-C", "Insumo C", 0, 0)
	elaboradoB := mustProducto(t, svc, admin, "ELAB-B", "Elaborado B", 0, 0)
	productoA := mustProducto(t, svc, admin, "ELAB-A", "Elaborado A", 0, 1000)

	mustCostoT08(t, svc, admin, ingX.ID, 50, "2026-10-01", nil)
	mustCostoT08(t, svc, admin, ingY.ID, 100, "2026-10-01", nil)
	mustCostoT08(t, svc, admin, ingC.ID, 100, "2026-10-01", nil)
	// B has its own purchase cost, but its recipe must take precedence.
	mustCostoT08(t, svc, admin, elaboradoB.ID, 999, "2026-10-01", nil)

	mustRecetaT08(t, svc, admin, elaboradoB.ID, nil, []RecetaItemInput{
		{IngredienteID: ingX.ID, Cantidad: 2},
		{IngredienteID: ingY.ID, Cantidad: 1},
	})
	bCosto := mustCostoMargenT08(t, svc, elaboradoB.ID)
	if bCosto.CostoBs == nil || *bCosto.CostoBs != 200 || bCosto.Origen != "receta" {
		t.Fatalf("elaborated B cost = %+v, want recursive recipe cost 200", bCosto)
	}

	mustRecetaT08(t, svc, admin, productoA.ID, nil, []RecetaItemInput{
		{IngredienteID: elaboradoB.ID, Cantidad: 2},
		{IngredienteID: ingC.ID, Cantidad: 1},
	})
	resultado := mustCostoMargenT08(t, svc, productoA.ID)
	if resultado.CostoBs == nil || *resultado.CostoBs != 500 ||
		resultado.MargenBs == nil || *resultado.MargenBs != 500 ||
		resultado.Origen != "receta" || !resultado.CostoCompleto {
		t.Fatalf("elaborated A cost/margin = %+v, want cost 500 margin 500", resultado)
	}
}

// AC6: an ingredient without cost and recipe yields an incomplete result (not
// zero), and a recipe cycle is rejected without partial rows.
func TestT08_AC6_CostoIncompletoYCiclos(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	_, _, admin, _ := seedT07(t, pool)
	ctx := context.Background()

	productoA := mustProducto(t, svc, admin, "CIC-A", "Producto A", 0, 1000)
	productoB := mustProducto(t, svc, admin, "CIC-B", "Producto B", 0, 0)
	ingC := mustProducto(t, svc, admin, "CIC-C", "Insumo C", 0, 0)
	mustCostoT08(t, svc, admin, productoB.ID, 200, "2026-10-01", nil)
	mustRecetaT08(t, svc, admin, productoA.ID, nil, []RecetaItemInput{
		{IngredienteID: productoB.ID, Cantidad: 2},
		{IngredienteID: ingC.ID, Cantidad: 1},
	})

	resultado := mustCostoMargenT08(t, svc, productoA.ID)
	if resultado.CostoCompleto || resultado.CostoBs != nil || resultado.MargenBs != nil ||
		resultado.Origen != "receta" {
		t.Fatalf("incomplete cost = %+v, want nil cost/margin", resultado)
	}
	if len(resultado.IngredientesSinCosto) != 1 ||
		resultado.IngredientesSinCosto[0].ProductoID != ingC.ID {
		t.Fatalf("missing-cost ingredients = %+v, want only C", resultado.IngredientesSinCosto)
	}

	// A -> B exists. Adding B -> A would close the cycle.
	if _, err := svc.UpdateReceta(ctx, productoB.ID, UpdateRecetaInput{
		UsuarioID: admin,
		Items:     []RecetaItemInput{{IngredienteID: productoA.ID, Cantidad: 1}},
	}); err == nil {
		t.Fatal("expected cycle rejection")
	} else {
		requireAppError(t, err, ErrRecetaCiclo.Status, ErrRecetaCiclo.Message)
	}
	var bDetalles int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM receta_detalles d JOIN recetas r ON r.id = d.receta_id
		 WHERE r.producto_elaborado_id = $1`, productoB.ID,
	).Scan(&bDetalles); err != nil {
		t.Fatal(err)
	}
	if bDetalles != 0 {
		t.Fatalf("B recipe details after cycle rejection = %d, want 0 (atomic)", bDetalles)
	}
	recetaA, err := svc.GetReceta(ctx, productoA.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireRecetaItemsT08(t, recetaA, map[int64]int64{productoB.ID: 2, ingC.ID: 1})
}

// AC7 / RF-30: selling the finished product never deducts recipe ingredients
// and never changes cost/margin.
func TestT08_AC7_RecetaNoDescuentaStock(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	emp, _, admin, _ := seedT07(t, pool)
	ctx := context.Background()

	productoA := mustProducto(t, svc, admin, "RF30-A", "Torta terminada", 0, 1000)
	ingB := mustProducto(t, svc, admin, "RF30-B", "Insumo B", 0, 0)
	ingC := mustProducto(t, svc, admin, "RF30-C", "Insumo C", 0, 0)
	mustCostoT08(t, svc, admin, ingB.ID, 200, "2026-10-01", nil)
	mustCostoT08(t, svc, admin, ingC.ID, 100, "2026-10-01", nil)
	mustRecetaT08(t, svc, admin, productoA.ID, nil, []RecetaItemInput{
		{IngredienteID: ingB.ID, Cantidad: 2},
		{IngredienteID: ingC.ID, Cantidad: 1},
	})

	deposito := mustDeposito(t, svc, admin, "Principal")
	mustEntrada(t, svc, admin, productoA.ID, deposito.ID, 10)
	mustEntrada(t, svc, admin, ingB.ID, deposito.ID, 10)
	mustEntrada(t, svc, admin, ingC.ID, deposito.ID, 10)

	before := mustCostoMargenT08(t, svc, productoA.ID)
	if before.CostoBs == nil || *before.CostoBs != 500 {
		t.Fatalf("cost before sale = %+v, want 500", before)
	}
	bMovs := movimientosCount(t, pool, ingB.ID)
	cMovs := movimientosCount(t, pool, ingC.ID)

	completion := completeByCard(t, svc, emp, []CreateSaleItem{
		{ProductoID: productoA.ID, Cantidad: 2},
	})
	if completion.Venta.Estado != "completada" {
		t.Fatalf("sale state = %s, want completada", completion.Venta.Estado)
	}

	if got := stockCantidad(t, pool, productoA.ID, deposito.ID); got != 8 {
		t.Fatalf("finished product stock = %d, want 8", got)
	}
	if got := stockCantidad(t, pool, ingB.ID, deposito.ID); got != 10 {
		t.Fatalf("ingredient B stock = %d, want unchanged 10", got)
	}
	if got := stockCantidad(t, pool, ingC.ID, deposito.ID); got != 10 {
		t.Fatalf("ingredient C stock = %d, want unchanged 10", got)
	}
	if got := movimientosCount(t, pool, ingB.ID); got != bMovs {
		t.Fatalf("ingredient B movements = %d, want unchanged %d", got, bMovs)
	}
	if got := movimientosCount(t, pool, ingC.ID); got != cMovs {
		t.Fatalf("ingredient C movements = %d, want unchanged %d", got, cMovs)
	}
	var ventasInsumos int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM movimientos_inventario
		 WHERE tipo = 'venta' AND producto_id IN ($1, $2)`, ingB.ID, ingC.ID,
	).Scan(&ventasInsumos); err != nil {
		t.Fatal(err)
	}
	if ventasInsumos != 0 {
		t.Fatalf("ingredient sale movements = %d, want 0", ventasInsumos)
	}

	after := mustCostoMargenT08(t, svc, productoA.ID)
	if after.CostoBs == nil || before.CostoBs == nil || *after.CostoBs != *before.CostoBs ||
		after.MargenBs == nil || before.MargenBs == nil || *after.MargenBs != *before.MargenBs {
		t.Fatalf("cost/margin changed after sale: before=%+v after=%+v", before, after)
	}
}

// AC8: only administrador/dueño may write costs and recipes; an employee gets
// 403 and nothing is persisted.
func TestT08_AC8_Autorizacion(t *testing.T) {
	svc, pool, cleanup := setupT07(t)
	defer cleanup()
	emp, _, admin, dueno := seedT07(t, pool)
	ctx := context.Background()

	producto := mustProducto(t, svc, admin, "ROL-T08", "Producto", 0, 1000)
	ingrediente := mustProducto(t, svc, admin, "ROL-ING", "Ingrediente", 0, 0)

	if _, err := svc.RegistrarCostoCompra(ctx, producto.ID, CostoCompraInput{
		UsuarioID: emp, CostoBs: 100, Fecha: "2026-10-01",
	}); err == nil {
		t.Fatal("expected employee cost rejection")
	} else {
		requireAppError(t, err, ErrNoAutorizado.Status, ErrNoAutorizado.Message)
	}
	if _, err := svc.UpdateReceta(ctx, producto.ID, UpdateRecetaInput{
		UsuarioID: emp,
		Items:     []RecetaItemInput{{IngredienteID: ingrediente.ID, Cantidad: 1}},
	}); err == nil {
		t.Fatal("expected employee recipe rejection")
	} else {
		requireAppError(t, err, ErrNoAutorizado.Status, ErrNoAutorizado.Message)
	}
	if got := countRowsT08(t, pool, `SELECT COUNT(*) FROM costos_compra`); got != 0 {
		t.Fatalf("cost rows after employee attempt = %d, want 0", got)
	}
	if got := countRowsT08(t, pool, `SELECT COUNT(*) FROM recetas`); got != 0 {
		t.Fatalf("recipe rows after employee attempt = %d, want 0", got)
	}

	for _, usuarioID := range []int64{admin, dueno} {
		mustCostoT08(t, svc, usuarioID, producto.ID, 100, "2026-10-01", nil)
		mustRecetaT08(t, svc, usuarioID, producto.ID, nil, []RecetaItemInput{
			{IngredienteID: ingrediente.ID, Cantidad: 1},
		})
	}
	receta, err := svc.GetReceta(ctx, producto.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireRecetaItemsT08(t, receta, map[int64]int64{ingrediente.ID: 1})
	if got := countRowsT08(t, pool, `SELECT COUNT(*) FROM costos_compra`); got != 2 {
		t.Fatalf("cost rows after admin/owner = %d, want 2", got)
	}
}

// Unit coverage for integer arithmetic without a database (RT-01).
func TestT08_AritmeticaEntera(t *testing.T) {
	linea, err := costoLinea(2, 200)
	if err != nil || linea != 400 {
		t.Fatalf("costoLinea(2,200) = %d, %v", linea, err)
	}
	if _, err := costoLinea(math.MaxInt64, 2); err == nil {
		t.Fatal("expected overflow guard on multiplication")
	}
	if _, err := costoLinea(0, 200); err == nil {
		t.Fatal("expected zero quantity rejection")
	}
	total, err := sumarCosto(300, 200)
	if err != nil || total != 500 {
		t.Fatalf("sumarCosto(300,200) = %d, %v", total, err)
	}
	if _, err := sumarCosto(math.MaxInt64, 1); err == nil {
		t.Fatal("expected overflow guard on sum")
	}
	margen := int64(500) - int64(700)
	if margen != -200 {
		t.Fatalf("negative margin = %d, want -200", margen)
	}
	missing := appendIngredienteSinCosto(nil, IngredienteSinCosto{ProductoID: 1})
	missing = appendIngredienteSinCosto(missing, IngredienteSinCosto{ProductoID: 1})
	if len(missing) != 1 {
		t.Fatalf("deduplicated missing ingredients = %+v, want 1", missing)
	}
}
