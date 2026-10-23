package pos

import (
	"fmt"
	"html"
	"strings"
)

// formatBs renders an integer amount in minor units as "Bs 12,34" without ever
// using floating point (RT-01).
func formatBs(monto int64) string {
	sign := ""
	if monto < 0 {
		sign = "-"
		monto = -monto
	}
	return fmt.Sprintf("%sBs %d,%02d", sign, monto/100, monto%100)
}

func copiaText(copia bool, numeroCopia int) string {
	if !copia {
		return ""
	}
	return fmt.Sprintf("*** COPIA (%d) ***", numeroCopia)
}

func copiaHTML(copia bool, numeroCopia int) string {
	if !copia {
		return ""
	}
	return fmt.Sprintf(`<div class="copia">*** COPIA (%d) ***</div>`, numeroCopia)
}

// RenderComandaTicket returns a print-ready ticket for one area. The browser
// can send the HTML directly to the print dialog (AC4); no hardware driver is
// involved. Reprints carry an explicit "COPIA" marker (AC3).
func RenderComandaTicket(comanda Comanda, copia bool, numeroCopia int, formato string) string {
	if formato == printFormatText {
		var b strings.Builder
		b.WriteString("================================\n")
		b.WriteString("COMANDAS\n")
		b.WriteString("================================\n")
		fmt.Fprintf(&b, "AREA: %s\n", comanda.AreaNombre)
		fmt.Fprintf(&b, "Comanda: #%d\n", comanda.ID)
		fmt.Fprintf(&b, "Venta: #%d\n", comanda.VentaID)
		fmt.Fprintf(&b, "Fecha: %s\n", comanda.CreadoEn.Format("2006-01-02 15:04"))
		b.WriteString("--------------------------------\n")
		b.WriteString("Cant  Producto\n")
		for _, item := range comanda.Items {
			fmt.Fprintf(&b, "%d x %s\n", item.Cantidad, item.Nombre)
		}
		b.WriteString("--------------------------------\n")
		if marker := copiaText(copia, numeroCopia); marker != "" {
			b.WriteString(marker)
			b.WriteString("\n")
		}
		return b.String()
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"es\"><head><meta charset=\"utf-8\">")
	fmt.Fprintf(&b, "<title>Comanda %d</title>", comanda.ID)
	b.WriteString(`<style>
body{font-family:ui-monospace,monospace;font-size:14px;margin:16px;color:#111}
h1{font-size:18px;margin:0 0 8px}
.meta{margin:0 0 12px;line-height:1.5}
table{width:100%;border-collapse:collapse}
td{padding:4px 0;vertical-align:top}
.cant{width:48px;font-weight:bold}
hr{border:0;border-top:1px dashed #555;margin:12px 0}
.copia{margin-top:12px;font-weight:bold;text-align:center;border:2px dashed #111;padding:8px}
@media print{body{margin:0}.no-print{display:none}}
</style></head><body>`)
	fmt.Fprintf(&b, `<h1>COMANDAS — %s</h1>`, html.EscapeString(comanda.AreaNombre))
	b.WriteString(`<div class="meta">`)
	fmt.Fprintf(&b, `<div>Comanda: #%d &nbsp; Venta: #%d</div>`, comanda.ID, comanda.VentaID)
	fmt.Fprintf(&b, `<div>Fecha: %s</div>`, html.EscapeString(comanda.CreadoEn.Format("2006-01-02 15:04")))
	b.WriteString(`</div><hr><table>`)
	for _, item := range comanda.Items {
		fmt.Fprintf(&b, `<tr><td class="cant">%d x</td><td>%s</td></tr>`,
			item.Cantidad, html.EscapeString(item.Nombre))
	}
	b.WriteString(`</table><hr>`)
	b.WriteString(copiaHTML(copia, numeroCopia))
	b.WriteString(`</body></html>`)
	return b.String()
}

// RenderReceipt returns a print-ready sale receipt. Amounts are already
// integer minor units and are only converted to text here (RT-01).
func RenderReceipt(sale Sale, atendidoPor string, copia bool, numeroCopia int, formato string) string {
	if formato == printFormatText {
		var b strings.Builder
		b.WriteString("================================\n")
		b.WriteString("BENGAL POS\n")
		b.WriteString("RECIBO DE VENTA\n")
		b.WriteString("================================\n")
		fmt.Fprintf(&b, "Venta: #%d\n", sale.ID)
		fmt.Fprintf(&b, "Fecha: %s\n", sale.Fecha.Format("2006-01-02 15:04"))
		if atendidoPor != "" {
			fmt.Fprintf(&b, "Atendido por: %s\n", atendidoPor)
		}
		b.WriteString("--------------------------------\n")
		for _, item := range sale.Items {
			fmt.Fprintf(&b, "%d x %s\n", item.Cantidad, item.ProductoNombre)
			fmt.Fprintf(&b, "    %s\n", formatBs(item.SubtotalBs))
		}
		b.WriteString("--------------------------------\n")
		subtotal := sale.TotalBs + sale.DescuentoBs
		fmt.Fprintf(&b, "Subtotal:  %s\n", formatBs(subtotal))
		fmt.Fprintf(&b, "Descuento: %s\n", formatBs(sale.DescuentoBs))
		fmt.Fprintf(&b, "TOTAL:     %s\n", formatBs(sale.TotalBs))
		b.WriteString("--------------------------------\n")
		b.WriteString("PAGOS\n")
		for _, part := range sale.Partes {
			fmt.Fprintf(&b, "%s: %s\n", part.MetodoNombre, formatBs(part.MontoBs))
		}
		b.WriteString("--------------------------------\n")
		if marker := copiaText(copia, numeroCopia); marker != "" {
			b.WriteString(marker)
			b.WriteString("\n")
		}
		return b.String()
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"es\"><head><meta charset=\"utf-8\">")
	fmt.Fprintf(&b, "<title>Recibo %d</title>", sale.ID)
	b.WriteString(`<style>
body{font-family:ui-monospace,monospace;font-size:14px;margin:16px;color:#111}
h1{font-size:18px;margin:0 0 8px}
.meta{margin:0 0 12px;line-height:1.5}
table{width:100%;border-collapse:collapse}
td{padding:4px 0;vertical-align:top}
.num{text-align:right;white-space:nowrap}
.total{font-weight:bold;font-size:16px}
hr{border:0;border-top:1px dashed #555;margin:12px 0}
.copia{margin-top:12px;font-weight:bold;text-align:center;border:2px dashed #111;padding:8px}
@media print{body{margin:0}.no-print{display:none}}
</style></head><body>`)
	b.WriteString(`<h1>BENGAL POS — RECIBO</h1><div class="meta">`)
	fmt.Fprintf(&b, `<div>Venta: #%d</div>`, sale.ID)
	fmt.Fprintf(&b, `<div>Fecha: %s</div>`, html.EscapeString(sale.Fecha.Format("2006-01-02 15:04")))
	if atendidoPor != "" {
		fmt.Fprintf(&b, `<div>Atendido por: %s</div>`, html.EscapeString(atendidoPor))
	}
	b.WriteString(`</div><hr><table>`)
	for _, item := range sale.Items {
		fmt.Fprintf(&b, `<tr><td>%d x %s</td><td class="num">%s</td></tr>`,
			item.Cantidad, html.EscapeString(item.ProductoNombre), formatBs(item.SubtotalBs))
	}
	b.WriteString(`</table><hr><table>`)
	fmt.Fprintf(&b, `<tr><td>Subtotal</td><td class="num">%s</td></tr>`, formatBs(sale.TotalBs+sale.DescuentoBs))
	fmt.Fprintf(&b, `<tr><td>Descuento</td><td class="num">%s</td></tr>`, formatBs(sale.DescuentoBs))
	fmt.Fprintf(&b, `<tr class="total"><td>TOTAL</td><td class="num">%s</td></tr>`, formatBs(sale.TotalBs))
	b.WriteString(`</table><hr><table>`)
	for _, part := range sale.Partes {
		fmt.Fprintf(&b, `<tr><td>%s</td><td class="num">%s</td></tr>`,
			html.EscapeString(part.MetodoNombre), formatBs(part.MontoBs))
	}
	b.WriteString(`</table><hr>`)
	b.WriteString(copiaHTML(copia, numeroCopia))
	b.WriteString(`</body></html>`)
	return b.String()
}
