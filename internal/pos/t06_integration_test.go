package pos

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupT06(t *testing.T) (*Service, *pgxpool.Pool, func()) {
	t.Helper()
	return setupT03(t)
}

func seedT06(t *testing.T, pool *pgxpool.Pool) (empleadoID, adminID, duenoID, productoID int64) {
	t.Helper()
	p := pool
	ctx := context.Background()
	if err := p.QueryRow(ctx, `INSERT INTO usuarios (nombre_completo, login, password_hash, rol) VALUES ('Empleado', 'emp1', 'h', 'empleado') ON CONFLICT (login) DO UPDATE SET nombre_completo = EXCLUDED.nombre_completo RETURNING id`).Scan(&empleadoID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO usuarios (nombre_completo, login, password_hash, rol) VALUES ('Admin', 'admin1', 'h', 'administrador') ON CONFLICT (login) DO UPDATE SET nombre_completo = EXCLUDED.nombre_completo RETURNING id`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO usuarios (nombre_completo, login, password_hash, rol) VALUES ('Dueño', 'dueno1', 'h', 'dueno') ON CONFLICT (login) DO UPDATE SET nombre_completo = EXCLUDED.nombre_completo RETURNING id`).Scan(&duenoID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO productos (codigo, nombre, precio_venta_bs, stock_minimo) VALUES ('P1', 'Producto 1', 10000, 0) ON CONFLICT (codigo) DO UPDATE SET nombre = EXCLUDED.nombre RETURNING id`).Scan(&productoID); err != nil {
		t.Fatal(err)
	}
	var depositoID int64
	if err := p.QueryRow(ctx, `INSERT INTO depositos (nombre) VALUES ('Principal') ON CONFLICT DO NOTHING RETURNING id`).Scan(&depositoID); err != nil {
		if err := p.QueryRow(ctx, `SELECT id FROM depositos ORDER BY id LIMIT 1`).Scan(&depositoID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, `INSERT INTO stock_depositos (producto_id, deposito_id, cantidad) VALUES ($1, $2, 100) ON CONFLICT (producto_id, deposito_id) DO UPDATE SET cantidad = EXCLUDED.cantidad`, productoID, depositoID); err != nil {
		t.Fatal(err)
	}
	return
}

func TestT06_AC1_OpenCloseBasics(t *testing.T) {
	svc, pool, cleanup := setupT06(t)
	defer cleanup()
	ctx := context.Background()
	_, admin, _, _ := seedT06(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 10000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err == nil {
		t.Fatal("expected duplicate open error")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Message != "Ya hay una caja abierta" {
			t.Fatalf("wrong error: %T %s", err, err)
		}
	}
	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: -1}); err == nil {
		t.Fatal("expected negative monto error")
	}
}

func TestT06_AC2_AC3_EntradasSalidas(t *testing.T) {
	svc, pool, cleanup := setupT06(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, _, _ := seedT06(t, pool)

	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "entrada", MontoBs: 100, Motivo: "x"}); err == nil {
		t.Fatal("expected no caja error")
	}
	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 10000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: emp, Tipo: "entrada", MontoBs: 100, Motivo: "x"}); err == nil {
		t.Fatal("expected no autorizado")
	}
	ent, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "entrada", MontoBs: 2500, Motivo: "Fondo adicional"})
	if err != nil {
		t.Fatal(err)
	}
	if ent.MontoBs != 2500 || ent.Tipo != "entrada" {
		t.Fatalf("bad entry: %+v", ent)
	}
	sal, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "salida", MontoBs: 1500, Motivo: "Compra de hielo"})
	if err != nil {
		t.Fatal(err)
	}
	if sal.MontoBs != -1500 || sal.Tipo != "salida" {
		t.Fatalf("bad exit: %+v", sal)
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "entrada", MontoBs: 0, Motivo: "x"}); err == nil {
		t.Fatal("bad monto")
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "salida", MontoBs: 100, Motivo: "   "}); err == nil {
		t.Fatal("bad motivo")
	}
}

func TestT06_AC4_CuadreCierre(t *testing.T) {
	svc, pool, cleanup := setupT06(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, _, prod := seedT06(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 10000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "entrada", MontoBs: 2500, Motivo: "Fondo adicional"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "salida", MontoBs: 1500, Motivo: "Compra de hielo"}); err != nil {
		t.Fatal(err)
	}
	// venta completada en efectivo 30000
	sale, err := svc.CreateSale(ctx, CreateSaleInput{UsuarioID: emp, Items: []CreateSaleItem{{ProductoID: prod, Cantidad: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE productos SET precio_venta_bs = 30000 WHERE id = $1`, prod); err != nil {
		t.Fatal(err)
	}
	sale, err = svc.CreateSale(ctx, CreateSaleInput{UsuarioID: emp, Items: []CreateSaleItem{{ProductoID: prod, Cantidad: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{UsuarioID: emp, MetodoCodigo: "efectivo", MontoBs: sale.TotalBs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, part.ID, emp); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.CloseCaja(ctx, CloseCajaInput{UsuarioID: admin, EfectivoContadoBs: 40500})
	if err != nil {
		t.Fatal(err)
	}
	if res.EfectivoEsperadoBs == nil || *res.EfectivoEsperadoBs != 41000 {
		t.Fatalf("esperado wrong: %+v", res.EfectivoEsperadoBs)
	}
	if res.EfectivoContadoBs == nil || *res.EfectivoContadoBs != 40500 {
		t.Fatalf("contado wrong: %+v", res.EfectivoContadoBs)
	}
	if res.DiferenciaBs == nil || *res.DiferenciaBs != -500 {
		t.Fatalf("diferencia wrong: %+v", res.DiferenciaBs)
	}
	if res.Estado != "cerrada" {
		t.Fatalf("estado: %s", res.Estado)
	}
	// after close, cannot add movements
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "entrada", MontoBs: 100, Motivo: "x"}); err == nil {
		t.Fatal("should not add after close")
	}
}

func TestT06_AC5_CierreConVentasEnProceso(t *testing.T) {
	svc, pool, cleanup := setupT06(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, _, prod := seedT06(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}
	// create venta en proceso with cash part confirmed? but to be "en proceso" with efectivo part confirmed yet not completed - AddPaymentPart adds pending; ConfirmPaymentPart confirms. Sale remains en_proceso until CompleteSale.
	sale, err := svc.CreateSale(ctx, CreateSaleInput{UsuarioID: emp, Items: []CreateSaleItem{{ProductoID: prod, Cantidad: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{UsuarioID: emp, MetodoCodigo: "efectivo", MontoBs: sale.TotalBs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, part.ID, emp); err != nil {
		t.Fatal(err)
	}

	res, err := svc.CloseCaja(ctx, CloseCajaInput{UsuarioID: admin, EfectivoContadoBs: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Aviso != avisoVentasEnProceso {
		t.Fatalf("aviso wrong: %s", res.Aviso)
	}
	if res.VentasEnProceso != 1 {
		t.Fatalf("ventas_en_proceso: %d", res.VentasEnProceso)
	}

	// now try to complete sale without open caja
	if _, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp}); err == nil {
		t.Fatal("expected error")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Message != "Debe abrir caja para cobrar en efectivo" {
			t.Fatalf("wrong error: %T %s", err, err)
		}
	}
}

func TestT06_AC6_Autorizacion(t *testing.T) {
	svc, pool, cleanup := setupT06(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, dueno, _ := seedT06(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: emp, MontoInicialBs: 0}); err == nil {
		t.Fatal("empleado cannot open")
	}
	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: emp, Tipo: "entrada", MontoBs: 100, Motivo: "x"}); err == nil {
		t.Fatal("empleado cannot move")
	}
	if _, err := svc.CloseCaja(ctx, CloseCajaInput{UsuarioID: emp, EfectivoContadoBs: 0}); err == nil {
		t.Fatal("empleado cannot close")
	}
	if _, err := svc.CloseCaja(ctx, CloseCajaInput{UsuarioID: admin, EfectivoContadoBs: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: dueno, MontoInicialBs: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: dueno, Tipo: "salida", MontoBs: 100, Motivo: "retiro"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloseCaja(ctx, CloseCajaInput{UsuarioID: dueno, EfectivoContadoBs: 0}); err != nil {
		t.Fatal(err)
	}
}

func TestT06_AC7_HistorialInmutable(t *testing.T) {
	svc, pool, cleanup := setupT06(t)
	defer cleanup()
	ctx := context.Background()
	emp, admin, _, prod := seedT06(t, pool)

	if _, err := svc.OpenCaja(ctx, OpenCajaInput{UsuarioID: admin, MontoInicialBs: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCajaMovimiento(ctx, CajaMovimientoInput{UsuarioID: admin, Tipo: "entrada", MontoBs: 500, Motivo: "extra"}); err != nil {
		t.Fatal(err)
	}
	sale, err := svc.CreateSale(ctx, CreateSaleInput{UsuarioID: emp, Items: []CreateSaleItem{{ProductoID: prod, Cantidad: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE productos SET precio_venta_bs = 2000 WHERE id = $1`, prod); err != nil {
		t.Fatal(err)
	}
	sale, err = svc.CreateSale(ctx, CreateSaleInput{UsuarioID: emp, Items: []CreateSaleItem{{ProductoID: prod, Cantidad: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	part, err := svc.AddPaymentPart(ctx, sale.ID, AddPaymentPartInput{UsuarioID: emp, MetodoCodigo: "efectivo", MontoBs: sale.TotalBs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmPaymentPart(ctx, sale.ID, part.ID, emp); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteSale(ctx, sale.ID, CompleteSaleInput{UsuarioID: emp}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloseCaja(ctx, CloseCajaInput{UsuarioID: admin, EfectivoContadoBs: 3500}); err != nil {
		t.Fatal(err)
	}

	sesiones, err := svc.ListCajaSesiones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sesiones) == 0 {
		t.Fatal("no sesiones")
	}
	movs, err := svc.ListCajaMovimientos(ctx, sesiones[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(movs) == 0 {
		t.Fatal("no movs")
	}
	if _, err := pool.Exec(ctx, `UPDATE caja_movimientos SET monto_bs = 0 WHERE id = $1`, movs[0].ID); err == nil {
		t.Fatal("mov update should fail")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM caja_movimientos WHERE id = $1`, movs[0].ID); err == nil {
		t.Fatal("mov delete should fail")
	}
	if _, err := pool.Exec(ctx, `UPDATE caja_sesiones SET monto_inicial_bs = 0 WHERE id = $1`, sesiones[0].ID); err == nil {
		t.Fatal("sesion update should fail")
	}
}
