package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bengalpos/internal/pos"

	"github.com/jung-kurt/gofpdf"
)

func (s *Server) requireReportAccess(w http.ResponseWriter, r *http.Request, scope string, targetUserID *int64) (*pos.UsuarioSesion, bool) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return nil, false
	}
	// dueño/administrador completo
	if u.Rol == "dueno" || u.Rol == "administrador" {
		return u, true
	}
	if u.Rol == "empleado" {
		// principle of least privilege: only own scope for ventas personales if requested? for general reports deny
		if scope == "ventas_propias" && targetUserID != nil && *targetUserID == u.ID {
			return u, true
		}
		if scope == "personal_propio" && targetUserID != nil && *targetUserID == u.ID {
			return u, true
		}
		writeError(w, pos.ErrNoAutorizado)
		return nil, false
	}
	if u.Rol == "encargado" {
		// restringido: try to be restrictive but allow own ventas? follow task: define explicitly, be restrictive by default
		if scope == "ventas_propias" && targetUserID != nil && *targetUserID == u.ID {
			return u, true
		}
		if scope == "personal_propio" && targetUserID != nil && *targetUserID == u.ID {
			return u, true
		}
		// por defecto restringido - global reports require dueño/admin
		writeError(w, pos.ErrNoAutorizado)
		return nil, false
	}
	writeError(w, pos.ErrNoAutorizado)
	return nil, false
}

func parseDateParam(r *http.Request, name string) (*time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, nil
	}
	// try date only
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return &t, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	return nil, &pos.AppError{Code: "parametro_invalido", Message: "Parámetro inválido: " + name, Status: http.StatusBadRequest}
}

func (s *Server) getReporteVentas(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	f := pos.ReporteVentasFiltro{}
	if v, err := queryInt64(r, "producto_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.ProductoID = v
	}
	if v, err := queryInt64(r, "usuario_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.UsuarioID = v
	}
	if v, err := queryInt64(r, "deposito_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.DepositoID = v
	}
	if d, err := parseDateParam(r, "desde"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Desde = d
	}
	if d, err := parseDateParam(r, "hasta"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Hasta = d
	}
	if m := strings.TrimSpace(r.URL.Query().Get("metodo")); m != "" {
		f.Metodo = &m
	}
	// authz
	if u.Rol != "dueno" && u.Rol != "administrador" {
		if u.Rol == "empleado" {
			if f.UsuarioID == nil || *f.UsuarioID != u.ID {
				writeError(w, pos.ErrNoAutorizado)
				return
			}
		} else if u.Rol == "encargado" {
			if f.UsuarioID == nil || *f.UsuarioID != u.ID {
				writeError(w, pos.ErrNoAutorizado)
				return
			}
		} else {
			writeError(w, pos.ErrNoAutorizado)
			return
		}
	}
	res, err := s.svc.ReportesVentas(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) getReporteInventario(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	if u.Rol != "dueno" && u.Rol != "administrador" {
		writeError(w, pos.ErrNoAutorizado)
		return
	}
	f := pos.ReporteInventarioFiltro{Tipo: strings.TrimSpace(r.URL.Query().Get("tipo"))}
	if v, err := queryInt64(r, "producto_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.ProductoID = v
	}
	if v, err := queryInt64(r, "deposito_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.DepositoID = v
	}
	if v, err := queryInt64(r, "tipo_mov"); err == nil && v == nil {
		if tm := strings.TrimSpace(r.URL.Query().Get("tipo_mov")); tm != "" {
			tmv := tm
			f.TipoMov = &tmv
		}
	} else if err != nil {
		writeError(w, err)
		return
	}
	if d, err := parseDateParam(r, "desde"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Desde = d
	}
	if d, err := parseDateParam(r, "hasta"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Hasta = d
	}
	res, err := s.svc.ReportesInventario(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) getReporteCaja(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	if u.Rol != "dueno" && u.Rol != "administrador" {
		writeError(w, pos.ErrNoAutorizado)
		return
	}
	f := pos.ReporteCajaFiltro{}
	if v, err := queryInt64(r, "sesion_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.SesionID = v
	}
	if v, err := queryInt64(r, "usuario_apertura_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.UsuarioAperturaID = v
	}
	if d, err := parseDateParam(r, "desde"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Desde = d
	}
	if d, err := parseDateParam(r, "hasta"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Hasta = d
	}
	res, err := s.svc.ReportesCaja(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) getReportePersonal(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	f := pos.ReportePersonalFiltro{Periodo: strings.TrimSpace(r.URL.Query().Get("periodo"))}
	if v, err := queryInt64(r, "usuario_id"); err != nil {
		writeError(w, err)
		return
	} else {
		f.UsuarioID = v
	}
	if d, err := parseDateParam(r, "desde"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Desde = d
	}
	if d, err := parseDateParam(r, "hasta"); err != nil {
		writeError(w, err)
		return
	} else {
		f.Hasta = d
	}
	if u.Rol != "dueno" && u.Rol != "administrador" {
		if u.Rol == "empleado" {
			if f.UsuarioID == nil || *f.UsuarioID != u.ID {
				writeError(w, pos.ErrNoAutorizado)
				return
			}
		} else if u.Rol == "encargado" {
			if f.UsuarioID == nil || *f.UsuarioID != u.ID {
				writeError(w, pos.ErrNoAutorizado)
				return
			}
		} else {
			writeError(w, pos.ErrNoAutorizado)
			return
		}
	}
	res, err := s.svc.ReportesPersonal(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) exportReporte(w http.ResponseWriter, r *http.Request) {
	tipo := strings.TrimSpace(r.PathValue("tipo"))
	formato := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if formato == "" {
		formato = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("formato")))
	}
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	switch tipo {
	case "ventas":
		if u.Rol != "dueno" && u.Rol != "administrador" && u.Rol != "empleado" && u.Rol != "encargado" {
			writeError(w, pos.ErrNoAutorizado)
			return
		}
		f := pos.ReporteVentasFiltro{}
		if v, err := queryInt64(r, "producto_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.ProductoID = v
		}
		if v, err := queryInt64(r, "usuario_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.UsuarioID = v
		}
		if v, err := queryInt64(r, "deposito_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.DepositoID = v
		}
		if d, err := parseDateParam(r, "desde"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Desde = d
		}
		if d, err := parseDateParam(r, "hasta"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Hasta = d
		}
		if m := strings.TrimSpace(r.URL.Query().Get("metodo")); m != "" {
			f.Metodo = &m
		}
		if u.Rol != "dueno" && u.Rol != "administrador" {
			if f.UsuarioID == nil || *f.UsuarioID != u.ID {
				writeError(w, pos.ErrNoAutorizado)
				return
			}
		}
		res, err := s.svc.ReportesVentas(r.Context(), f)
		if err != nil {
			writeError(w, err)
			return
		}
		if formato == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-ventas.csv")
			cw := csv.NewWriter(w)
			_ = cw.Write([]string{"id_venta", "fecha", "id_usuario", "nombre_usuario", "id_producto", "codigo_producto", "nombre_producto", "cantidad", "precio_unitario_bs", "descuento_bs", "subtotal_bs", "metodo_pago"})
			for _, it := range res.Detalle {
				_ = cw.Write([]string{
					strconv.FormatInt(it.VentaID, 10),
					it.Fecha.Format(time.RFC3339),
					strconv.FormatInt(it.UsuarioID, 10),
					it.UsuarioNombre,
					strconv.FormatInt(it.ProductoID, 10),
					it.ProductoCodigo,
					it.ProductoNombre,
					strconv.FormatInt(it.Cantidad, 10),
					strconv.FormatInt(it.PrecioUnitarioBs, 10),
					strconv.FormatInt(it.DescuentoBs, 10),
					strconv.FormatInt(it.SubtotalBs, 10),
					it.MetodoPago,
				})
			}
			_ = cw.Write([]string{"TOTALES", "", "", "", "", "", "",
				strconv.FormatInt(res.Totales.CantidadItems, 10),
				"", strconv.FormatInt(res.Totales.DescuentoBs, 10),
				strconv.FormatInt(res.Totales.TotalBs, 10), ""})
			cw.Flush()
			return
		}
		if formato == "pdf" {
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 14)
			pdf.Cell(40, 10, "Reporte de Ventas")
			pdf.Ln(12)
			pdf.SetFont("Arial", "", 8)
			pdf.CellFormat(190, 8, fmt.Sprintf("Generado: %s | Cantidad ventas: %d | Total Bs: %d", res.GeneradoEn.Format(time.RFC3339), res.Totales.CantidadVentas, res.Totales.TotalBs), "", 1, "L", false, 0, "")
			pdf.Ln(6)
			for _, it := range res.Detalle {
				line := fmt.Sprintf("V#%d %s %s %s x%d @%d sub:%d", it.VentaID, it.Fecha.Format("2006-01-02T15:04:05"), it.ProductoCodigo, it.UsuarioNombre, it.Cantidad, it.PrecioUnitarioBs, it.SubtotalBs)
				pdf.MultiCell(190, 4, line, "", "L", false)
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-ventas.pdf")
			_ = pdf.Output(w)
			return
		}
		writeError(w, &pos.AppError{Code: "formato_invalido", Message: "Formato debe ser csv o pdf", Status: http.StatusBadRequest})
		return
	case "inventario":
		if u.Rol != "dueno" && u.Rol != "administrador" {
			writeError(w, pos.ErrNoAutorizado)
			return
		}
		f := pos.ReporteInventarioFiltro{Tipo: strings.TrimSpace(r.URL.Query().Get("tipo"))}
		if v, err := queryInt64(r, "producto_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.ProductoID = v
		}
		if v, err := queryInt64(r, "deposito_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.DepositoID = v
		}
		if tm := strings.TrimSpace(r.URL.Query().Get("tipo_mov")); tm != "" {
			f.TipoMov = &tm
		}
		if d, err := parseDateParam(r, "desde"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Desde = d
		}
		if d, err := parseDateParam(r, "hasta"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Hasta = d
		}
		res, err := s.svc.ReportesInventario(r.Context(), f)
		if err != nil {
			writeError(w, err)
			return
		}
		if formato == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-inventario.csv")
			cw := csv.NewWriter(w)
			if f.Tipo == "movimientos" {
				_ = cw.Write([]string{"id", "id_producto", "codigo_producto", "nombre_producto", "id_deposito", "nombre_deposito", "tipo", "cantidad", "id_venta", "id_usuario", "motivo", "creado_en"})
				for _, it := range res.Movimientos {
					vid := ""
					if it.VentaID != nil {
						vid = strconv.FormatInt(*it.VentaID, 10)
					}
					mot := ""
					if it.Motivo != nil {
						mot = *it.Motivo
					}
					_ = cw.Write([]string{strconv.FormatInt(it.ID, 10), strconv.FormatInt(it.ProductoID, 10), it.ProductoCodigo, it.ProductoNombre, strconv.FormatInt(it.DepositoID, 10), it.DepositoNombre, it.Tipo, strconv.FormatInt(it.Cantidad, 10), vid, strconv.FormatInt(it.UsuarioID, 10), mot, it.CreadoEn.Format(time.RFC3339)})
				}
			} else {
				items := res.StockActual
				if f.Tipo == "bajo_minimo" {
					items = res.BajoMinimo
				}
				_ = cw.Write([]string{"producto_id", "producto_codigo", "producto_nombre", "stock_minimo", "stock_total", "bajo_minimo", "proveedor_principal_id", "proveedor_principal_nombre"})
				for _, it := range items {
					pid := ""
					if it.ProveedorPrincipalID != nil {
						pid = strconv.FormatInt(*it.ProveedorPrincipalID, 10)
					}
					_ = cw.Write([]string{strconv.FormatInt(it.ProductoID, 10), it.ProductoCodigo, it.ProductoNombre, strconv.FormatInt(it.StockMinimo, 10), strconv.FormatInt(it.StockTotal, 10), strconv.FormatBool(it.BajoMinimo), pid, it.ProveedorPrincipalNombre})
				}
			}
			cw.Flush()
			return
		}
		if formato == "pdf" {
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 14)
			pdf.Cell(40, 10, "Reporte de Inventario")
			pdf.Ln(12)
			pdf.SetFont("Arial", "", 8)
			pdf.MultiCell(190, 4, fmt.Sprintf("Generado: %s Tipo: %s", res.GeneradoEn.Format(time.RFC3339), f.Tipo), "", "L", false)
			_ = pdf.Output(w)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-inventario.pdf")
			return
		}
		writeError(w, &pos.AppError{Code: "formato_invalido", Message: "Formato debe ser csv o pdf", Status: http.StatusBadRequest})
		return
	case "caja":
		if u.Rol != "dueno" && u.Rol != "administrador" {
			writeError(w, pos.ErrNoAutorizado)
			return
		}
		f := pos.ReporteCajaFiltro{}
		if v, err := queryInt64(r, "sesion_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.SesionID = v
		}
		if v, err := queryInt64(r, "usuario_apertura_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.UsuarioAperturaID = v
		}
		if d, err := parseDateParam(r, "desde"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Desde = d
		}
		if d, err := parseDateParam(r, "hasta"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Hasta = d
		}
		res, err := s.svc.ReportesCaja(r.Context(), f)
		if err != nil {
			writeError(w, err)
			return
		}
		if formato == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-caja.csv")
			cw := csv.NewWriter(w)
			_ = cw.Write([]string{"id_sesion", "estado", "fecha_apertura", "id_usuario_apertura", "monto_inicial_bs", "ventas_efectivo_bs", "entradas_bs", "salidas_bs", "esperado_bs", "contado_bs", "diferencia_bs"})
			cb := ""
			dbs := ""
			if res.ContadoBs != nil {
				cb = strconv.FormatInt(*res.ContadoBs, 10)
			}
			if res.DiferenciaBs != nil {
				dbs = strconv.FormatInt(*res.DiferenciaBs, 10)
			}
			_ = cw.Write([]string{strconv.FormatInt(res.Sesion.ID, 10), res.Sesion.Estado, res.Sesion.FechaApertura.Format(time.RFC3339), strconv.FormatInt(res.Sesion.UsuarioAperturaID, 10), strconv.FormatInt(res.Sesion.MontoInicialBs, 10), strconv.FormatInt(res.VentasEfectivoBs, 10), strconv.FormatInt(res.EntradasBs, 10), strconv.FormatInt(res.SalidasBs, 10), strconv.FormatInt(res.EsperadoBs, 10), cb, dbs})
			cw.Flush()
			return
		}
		if formato == "pdf" {
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 14)
			pdf.Cell(40, 10, "Reporte de Caja")
			pdf.Ln(12)
			pdf.SetFont("Arial", "", 9)
			pdf.CellFormat(190, 8, fmt.Sprintf("Sesion %d Estado %s Apertura %s MontoIni %d Esperado %d", res.Sesion.ID, res.Sesion.Estado, res.Sesion.FechaApertura.Format(time.RFC3339), res.Sesion.MontoInicialBs, res.EsperadoBs), "", 1, "L", false, 0, "")
			_ = pdf.Output(w)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-caja.pdf")
			return
		}
		writeError(w, &pos.AppError{Code: "formato_invalido", Message: "Formato debe ser csv o pdf", Status: http.StatusBadRequest})
		return
	case "personal":
		f := pos.ReportePersonalFiltro{Periodo: strings.TrimSpace(r.URL.Query().Get("periodo"))}
		if v, err := queryInt64(r, "usuario_id"); err != nil {
			writeError(w, err)
			return
		} else {
			f.UsuarioID = v
		}
		if d, err := parseDateParam(r, "desde"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Desde = d
		}
		if d, err := parseDateParam(r, "hasta"); err != nil {
			writeError(w, err)
			return
		} else {
			f.Hasta = d
		}
		if u.Rol != "dueno" && u.Rol != "administrador" {
			if u.Rol == "empleado" {
				if f.UsuarioID == nil || *f.UsuarioID != u.ID {
					writeError(w, pos.ErrNoAutorizado)
					return
				}
			} else if u.Rol == "encargado" {
				if f.UsuarioID == nil || *f.UsuarioID != u.ID {
					writeError(w, pos.ErrNoAutorizado)
					return
				}
			} else {
				writeError(w, pos.ErrNoAutorizado)
				return
			}
		}
		res, err := s.svc.ReportesPersonal(r.Context(), f)
		if err != nil {
			writeError(w, err)
			return
		}
		if formato == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-personal.csv")
			cw := csv.NewWriter(w)
			_ = cw.Write([]string{"tipo", "id", "id_usuario", "nombre_usuario", "fecha", "detalle", "monto_bs"})
			for _, a := range res.Asistencia {
				mot := ""
				if a.Motivo != nil {
					mot = *a.Motivo
				}
				_ = cw.Write([]string{"asistencia", strconv.FormatInt(a.ID, 10), strconv.FormatInt(a.UsuarioID, 10), a.UsuarioNombre, a.FechaHora.Format(time.RFC3339), a.Tipo + " " + mot, ""})
			}
			for _, p := range res.Pagos {
				_ = cw.Write([]string{"pago", strconv.FormatInt(p.ID, 10), strconv.FormatInt(p.UsuarioID, 10), p.UsuarioNombre, p.FechaPago, p.Concepto, strconv.FormatInt(p.MontoBs, 10)})
			}
			cw.Flush()
			return
		}
		if formato == "pdf" {
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 14)
			pdf.Cell(40, 10, "Reporte de Personal - Asistencia/Pagos")
			pdf.Ln(12)
			pdf.SetFont("Arial", "", 8)
			pdf.CellFormat(190, 8, fmt.Sprintf("Generado: %s Total pagos Bs: %d", res.GeneradoEn.Format(time.RFC3339), res.TotalPagosBs), "", 1, "L", false, 0, "")
			_ = pdf.Output(w)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=reporte-personal.pdf")
			return
		}
		writeError(w, &pos.AppError{Code: "formato_invalido", Message: "Formato debe ser csv o pdf", Status: http.StatusBadRequest})
		return
	default:
		writeError(w, &pos.AppError{Code: "reporte_no_encontrado", Message: "Tipo de reporte no soportado", Status: http.StatusNotFound})
		return
	}
}
