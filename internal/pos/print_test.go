package pos

import (
	"strings"
	"testing"
	"time"
)

func sampleComanda() Comanda {
	return Comanda{
		ID:                7,
		VentaID:           12,
		AreaPreparacionID: 1,
		AreaNombre:        "Cocina",
		CreadoEn:          time.Date(2026, 10, 23, 14, 3, 0, 0, time.UTC),
		Items: []ComandaItem{
			{ID: 1, ComandaID: 7, VentaDetalleID: 1, ProductoID: 10, Cantidad: 2, Nombre: "Hamburguesa"},
			{ID: 2, ComandaID: 7, VentaDetalleID: 2, ProductoID: 11, Cantidad: 1, Nombre: "Papas fritas"},
		},
	}
}

func TestRenderComandaTicketOriginalAndCopy(t *testing.T) {
	original := RenderComandaTicket(sampleComanda(), false, 1, printFormatText)
	if !strings.Contains(original, "COMANDAS") || !strings.Contains(original, "AREA: Cocina") {
		t.Fatalf("ticket missing header: %s", original)
	}
	if !strings.Contains(original, "2 x Hamburguesa") || !strings.Contains(original, "1 x Papas fritas") {
		t.Fatalf("ticket missing items: %s", original)
	}
	if strings.Contains(original, "COPIA") {
		t.Fatalf("original ticket must not be marked as copy: %s", original)
	}

	copia := RenderComandaTicket(sampleComanda(), true, 2, printFormatText)
	if !strings.Contains(copia, "*** COPIA (2) ***") {
		t.Fatalf("reprint missing copy marker: %s", copia)
	}
}

func TestRenderComandaTicketHTML(t *testing.T) {
	comanda := sampleComanda()
	comanda.AreaNombre = `Barra & <café>`
	out := RenderComandaTicket(comanda, true, 3, printFormatHTML)
	if !strings.Contains(out, "<!doctype html>") || !strings.Contains(out, "@media print") {
		t.Fatalf("not print-ready html: %.120s", out)
	}
	if !strings.Contains(out, "Barra &amp; &lt;café&gt;") {
		t.Fatalf("area name not escaped: %s", out)
	}
	if !strings.Contains(out, "COPIA (3)") {
		t.Fatalf("html reprint missing copy marker: %s", out)
	}
}

func sampleSale() Sale {
	return Sale{
		ID:          12,
		Estado:      "completada",
		Fecha:       time.Date(2026, 10, 23, 14, 3, 0, 0, time.UTC),
		TotalBs:     10000,
		DescuentoBs: 500,
		Items: []SaleLine{
			{ID: 1, ProductoID: 10, ProductoNombre: "Hamburguesa", Cantidad: 2, SubtotalBs: 9000},
			{ID: 2, ProductoID: 11, ProductoNombre: "Papas fritas", Cantidad: 1, SubtotalBs: 1500},
		},
		Partes: []PaymentPart{
			{ID: 1, MetodoNombre: "Efectivo", MontoBs: 6000},
			{ID: 2, MetodoNombre: "Tarjeta", MontoBs: 4000},
		},
	}
}

func TestRenderReceiptTotalsAndCopy(t *testing.T) {
	original := RenderReceipt(sampleSale(), "Ana", false, 1, printFormatText)
	for _, want := range []string{"RECIBO DE VENTA", "Atendido por: Ana", "TOTAL:     Bs 100,00", "Bs 105,00", "Bs 5,00"} {
		if !strings.Contains(original, want) {
			t.Fatalf("receipt missing %q:\n%s", want, original)
		}
	}
	if strings.Contains(original, "COPIA") {
		t.Fatalf("original receipt must not be marked as copy")
	}

	copia := RenderReceipt(sampleSale(), "Ana", true, 2, printFormatText)
	if !strings.Contains(copia, "*** COPIA (2) ***") {
		t.Fatalf("receipt reprint missing copy marker:\n%s", copia)
	}
}

func TestRenderReceiptHTML(t *testing.T) {
	out := RenderReceipt(sampleSale(), "Ana", false, 1, printFormatHTML)
	if !strings.Contains(out, "<!doctype html>") || !strings.Contains(out, "TOTAL") {
		t.Fatalf("not print-ready receipt html: %.160s", out)
	}
}

func TestFormatBsIntegerMinorUnits(t *testing.T) {
	cases := map[int64]string{
		0:     "Bs 0,00",
		5:     "Bs 0,05",
		50:    "Bs 0,50",
		4550:  "Bs 45,50",
		10000: "Bs 100,00",
		-500:  "-Bs 5,00",
	}
	for amount, want := range cases {
		if got := formatBs(amount); got != want {
			t.Fatalf("formatBs(%d) = %q, want %q", amount, got, want)
		}
	}
}
