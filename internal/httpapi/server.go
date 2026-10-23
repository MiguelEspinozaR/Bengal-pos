// Package httpapi exposes the sales and cash use cases as a small JSON REST
// API. Validation errors use the business messages from docs/reglas-de-negocio.md.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bengalpos/internal/pos"
)

// Server wires HTTP handlers to the pos service.
type Server struct {
	svc *pos.Service
}

// NewServer returns the complete API handler.
func NewServer(svc *pos.Service) http.Handler {
	s := &Server{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /api/caja/apertura", s.openCaja)
	mux.HandleFunc("POST /api/caja/cierre", s.closeCaja)
	mux.HandleFunc("GET /api/caja/abierta", s.getOpenCaja)
	mux.HandleFunc("POST /api/caja/movimientos", s.addCajaMovimiento)
	mux.HandleFunc("POST /api/caja/entradas", s.addCajaEntrada)
	mux.HandleFunc("POST /api/caja/salidas", s.addCajaSalida)
	mux.HandleFunc("GET /api/caja/historial", s.listCajaSesiones)
	mux.HandleFunc("GET /api/caja/sesiones", s.listCajaSesiones)
	mux.HandleFunc("GET /api/caja/sesiones/{id}/movimientos", s.listCajaMovimientos)
	mux.HandleFunc("GET /api/caja/historial/{id}/movimientos", s.listCajaMovimientos)
	mux.HandleFunc("POST /api/ventas", s.createSale)
	mux.HandleFunc("GET /api/ventas/{id}", s.getSale)
	mux.HandleFunc("POST /api/ventas/{id}/partes", s.addPaymentPart)
	mux.HandleFunc("POST /api/ventas/{id}/partes/{parteID}/confirmar", s.confirmPaymentPart)
	mux.HandleFunc("POST /api/ventas/{id}/completar", s.completeSale)
	mux.HandleFunc("GET /api/ventas/{id}/comandas", s.listComandas)
	mux.HandleFunc("POST /api/ventas/{id}/recibo/imprimir", s.printReceipt)
	mux.HandleFunc("POST /api/comandas/{id}/imprimir", s.printComanda)
	mux.HandleFunc("GET /api/areas", s.listAreas)
	mux.HandleFunc("POST /api/areas", s.createArea)
	mux.HandleFunc("PUT /api/areas/{id}", s.updateArea)
	mux.HandleFunc("PUT /api/productos/{id}/area", s.assignProductArea)

	// T-07: catalog, suppliers, deposits and inventory (RF-01..RF-08).
	mux.HandleFunc("POST /api/productos", s.createProducto)
	mux.HandleFunc("GET /api/productos", s.listProductos)
	mux.HandleFunc("GET /api/productos/{id}", s.getProducto)
	mux.HandleFunc("PUT /api/productos/{id}", s.updateProducto)
	mux.HandleFunc("GET /api/productos/{id}/proveedores", s.listProductoProveedores)
	mux.HandleFunc("POST /api/productos/{id}/proveedores", s.addProductoProveedor)
	mux.HandleFunc("DELETE /api/productos/{id}/proveedores/{proveedorID}", s.removeProductoProveedor)
	mux.HandleFunc("GET /api/proveedores", s.listProveedores)
	mux.HandleFunc("POST /api/proveedores", s.createProveedor)
	mux.HandleFunc("PUT /api/proveedores/{id}", s.updateProveedor)
	mux.HandleFunc("GET /api/depositos", s.listDepositos)
	mux.HandleFunc("POST /api/depositos", s.createDeposito)
	mux.HandleFunc("PUT /api/depositos/{id}", s.updateDeposito)
	mux.HandleFunc("GET /api/inventario/stock", s.getStock)
	mux.HandleFunc("GET /api/inventario/movimientos", s.listMovimientos)
	mux.HandleFunc("POST /api/inventario/movimientos", s.addMovimiento)
	mux.HandleFunc("POST /api/inventario/traslados", s.transferirStock)
	mux.HandleFunc("GET /api/inventario/bajo-stock", s.bajoStock)

	// T-08: purchase costs, recipes and margin (RF-27..RF-30).
	mux.HandleFunc("POST /api/productos/{id}/costos", s.addCostoCompra)
	mux.HandleFunc("GET /api/productos/{id}/costos", s.listCostosProducto)
	mux.HandleFunc("GET /api/productos/{id}/receta", s.getReceta)
	mux.HandleFunc("PUT /api/productos/{id}/receta", s.updateReceta)
	mux.HandleFunc("GET /api/productos/{id}/costo-margen", s.getCostoMargen)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) openCaja(w http.ResponseWriter, r *http.Request) {
	var in pos.OpenCajaInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	caja, err := s.svc.OpenCaja(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, caja)
}

func (s *Server) closeCaja(w http.ResponseWriter, r *http.Request) {
	var in pos.CloseCajaInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.svc.CloseCaja(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) addCajaMovimiento(w http.ResponseWriter, r *http.Request) {
	var in pos.CajaMovimientoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	movimiento, err := s.svc.AddCajaMovimiento(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, movimiento)
}

func (s *Server) addCajaEntrada(w http.ResponseWriter, r *http.Request) {
	var in pos.CajaMovimientoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.Tipo = "entrada"
	movimiento, err := s.svc.AddCajaMovimiento(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, movimiento)
}

func (s *Server) addCajaSalida(w http.ResponseWriter, r *http.Request) {
	var in pos.CajaMovimientoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.Tipo = "salida"
	movimiento, err := s.svc.AddCajaMovimiento(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, movimiento)
}

func (s *Server) listCajaSesiones(w http.ResponseWriter, r *http.Request) {
	sesiones, err := s.svc.ListCajaSesiones(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sesiones": sesiones})
}

func (s *Server) listCajaMovimientos(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	movimientos, err := s.svc.ListCajaMovimientos(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"movimientos": movimientos})
}

func (s *Server) getOpenCaja(w http.ResponseWriter, r *http.Request) {
	caja, err := s.svc.GetOpenCaja(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"caja": caja})
}

func (s *Server) createSale(w http.ResponseWriter, r *http.Request) {
	var in pos.CreateSaleInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	sale, err := s.svc.CreateSale(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sale)
}

func (s *Server) getSale(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	sale, err := s.svc.GetSale(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sale)
}

func (s *Server) addPaymentPart(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.AddPaymentPartInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	part, err := s.svc.AddPaymentPart(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, part)
}

type confirmPartRequest struct {
	UsuarioID int64 `json:"usuario_id"`
}

func (s *Server) confirmPaymentPart(w http.ResponseWriter, r *http.Request) {
	ventaID, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	partID, err := pathID(r, "parteID")
	if err != nil {
		writeError(w, err)
		return
	}
	var in confirmPartRequest
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	part, err := s.svc.ConfirmPaymentPart(r.Context(), ventaID, partID, in.UsuarioID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, part)
}

type completeSaleRequest struct {
	UsuarioID              int64  `json:"usuario_id"`
	DepositoID             *int64 `json:"deposito_id"`
	ConfirmarStockNegativo bool   `json:"confirmar_stock_negativo"`
}

func (s *Server) completeSale(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in completeSaleRequest
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	completion, err := s.svc.CompleteSale(r.Context(), id, pos.CompleteSaleInput{
		UsuarioID:              in.UsuarioID,
		IdempotencyKey:         strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		DepositoID:             in.DepositoID,
		ConfirmarStockNegativo: in.ConfirmarStockNegativo,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, completion)
}

func (s *Server) listComandas(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	comandas, err := s.svc.ListComandas(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comandas": comandas})
}

type printRequest struct {
	UsuarioID int64 `json:"usuario_id"`
}

func (s *Server) printComanda(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in printRequest
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	doc, err := s.svc.PrintComanda(r.Context(), id, in.UsuarioID, r.URL.Query().Get("formato"))
	if err != nil {
		writeError(w, err)
		return
	}
	writePrintDocument(w, doc)
}

func (s *Server) printReceipt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in printRequest
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	doc, err := s.svc.PrintReceipt(r.Context(), id, in.UsuarioID, r.URL.Query().Get("formato"))
	if err != nil {
		writeError(w, err)
		return
	}
	writePrintDocument(w, doc)
}

// writePrintDocument returns the browser-printable ticket directly, with the
// audit metadata in headers so the client can confirm original vs copy.
func writePrintDocument(w http.ResponseWriter, doc *pos.PrintDocument) {
	contentType := "text/plain; charset=utf-8"
	if doc.Formato == "html" {
		contentType = "text/html; charset=utf-8"
	}
	copia := "false"
	if doc.Impresion.Copia {
		copia = "true"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Impresion-Id", strconv.FormatInt(doc.Impresion.ID, 10))
	w.Header().Set("X-Numero-Copia", strconv.Itoa(doc.Impresion.NumeroCopia))
	w.Header().Set("X-Copia", copia)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(doc.Contenido)); err != nil {
		log.Printf("write print document: %v", err)
	}
}

func (s *Server) listAreas(w http.ResponseWriter, r *http.Request) {
	areas, err := s.svc.ListAreas(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"areas": areas})
}

func (s *Server) createArea(w http.ResponseWriter, r *http.Request) {
	var in pos.CreateAreaInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	area, err := s.svc.CreateArea(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, area)
}

func (s *Server) updateArea(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.UpdateAreaInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	area, err := s.svc.UpdateArea(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, area)
}

func (s *Server) assignProductArea(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.AssignProductAreaInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	product, err := s.svc.AssignProductArea(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) createProducto(w http.ResponseWriter, r *http.Request) {
	var in pos.CreateProductoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	producto, err := s.svc.CreateProducto(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, producto)
}

func (s *Server) listProductos(w http.ResponseWriter, r *http.Request) {
	incluir, err := queryBool(r, "incluir_inactivos")
	if err != nil {
		writeError(w, err)
		return
	}
	productos, err := s.svc.ListProductos(r.Context(), incluir)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"productos": productos})
}

func (s *Server) getProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	producto, err := s.svc.GetProducto(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, producto)
}

func (s *Server) updateProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.UpdateProductoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	producto, err := s.svc.UpdateProducto(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, producto)
}

func (s *Server) listProductoProveedores(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	asociaciones, err := s.svc.ListProductoProveedores(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proveedores": asociaciones})
}

func (s *Server) addProductoProveedor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.AsociarProveedorInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	asociacion, err := s.svc.AsociarProductoProveedor(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, asociacion)
}

func (s *Server) removeProductoProveedor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	proveedorID, err := pathID(r, "proveedorID")
	if err != nil {
		writeError(w, err)
		return
	}
	usuarioID, err := deleteUsuarioID(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.svc.RemoveProductoProveedor(r.Context(), id, proveedorID, usuarioID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addCostoCompra(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.CostoCompraInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	costo, err := s.svc.RegistrarCostoCompra(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, costo)
}

func (s *Server) listCostosProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	costos, err := s.svc.ListCostosProducto(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"costos": costos})
}

func (s *Server) getReceta(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	receta, err := s.svc.GetReceta(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receta)
}

func (s *Server) updateReceta(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.UpdateRecetaInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	receta, err := s.svc.UpdateReceta(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receta)
}

func (s *Server) getCostoMargen(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	resultado, err := s.svc.CostoMargen(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resultado)
}

func (s *Server) listProveedores(w http.ResponseWriter, r *http.Request) {
	incluir, err := queryBool(r, "incluir_inactivos")
	if err != nil {
		writeError(w, err)
		return
	}
	proveedores, err := s.svc.ListProveedores(r.Context(), incluir)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proveedores": proveedores})
}

func (s *Server) createProveedor(w http.ResponseWriter, r *http.Request) {
	var in pos.CreateProveedorInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	proveedor, err := s.svc.CreateProveedor(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, proveedor)
}

func (s *Server) updateProveedor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.UpdateProveedorInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	proveedor, err := s.svc.UpdateProveedor(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, proveedor)
}

func (s *Server) listDepositos(w http.ResponseWriter, r *http.Request) {
	incluir, err := queryBool(r, "incluir_inactivos")
	if err != nil {
		writeError(w, err)
		return
	}
	depositos, err := s.svc.ListDepositos(r.Context(), incluir)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"depositos": depositos})
}

func (s *Server) createDeposito(w http.ResponseWriter, r *http.Request) {
	var in pos.CreateDepositoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	deposito, err := s.svc.CreateDeposito(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deposito)
}

func (s *Server) updateDeposito(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.UpdateDepositoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	deposito, err := s.svc.UpdateDeposito(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deposito)
}

func (s *Server) getStock(w http.ResponseWriter, r *http.Request) {
	productoID, err := queryInt64(r, "producto_id")
	if err != nil {
		writeError(w, err)
		return
	}
	depositoID, err := queryInt64(r, "deposito_id")
	if err != nil {
		writeError(w, err)
		return
	}
	stock, err := s.svc.GetStock(r.Context(), productoID, depositoID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stock": stock})
}

func (s *Server) listMovimientos(w http.ResponseWriter, r *http.Request) {
	productoID, err := queryInt64(r, "producto_id")
	if err != nil {
		writeError(w, err)
		return
	}
	depositoID, err := queryInt64(r, "deposito_id")
	if err != nil {
		writeError(w, err)
		return
	}
	movimientos, err := s.svc.ListMovimientos(r.Context(), productoID, depositoID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"movimientos": movimientos})
}

func (s *Server) addMovimiento(w http.ResponseWriter, r *http.Request) {
	var in pos.InventarioMovimientoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	resultado, err := s.svc.AddMovimientoInventario(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resultado)
}

func (s *Server) transferirStock(w http.ResponseWriter, r *http.Request) {
	var in pos.TrasladoInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	resultado, err := s.svc.TransferirStock(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resultado)
}

func (s *Server) bajoStock(w http.ResponseWriter, r *http.Request) {
	reporte, err := s.svc.BajoStock(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reporte)
}

func queryBool(r *http.Request, name string) (bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, &pos.AppError{Code: "parametro_invalido", Message: "Parámetro inválido: " + name, Status: http.StatusBadRequest}
	}
	return value, nil
}

func queryInt64(r *http.Request, name string) (*int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, &pos.AppError{Code: "parametro_invalido", Message: "Parámetro inválido: " + name, Status: http.StatusBadRequest}
	}
	return &value, nil
}

// deleteUsuarioID reads the actor from ?usuario_id= or, when absent, from a
// small JSON body. DELETE has no other payload.
func deleteUsuarioID(w http.ResponseWriter, r *http.Request) (int64, error) {
	if raw := strings.TrimSpace(r.URL.Query().Get("usuario_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, &pos.AppError{Code: "parametro_invalido", Message: "Parámetro inválido: usuario_id", Status: http.StatusBadRequest}
		}
		return id, nil
	}
	var in struct {
		UsuarioID int64 `json:"usuario_id"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return 0, err
	}
	return in.UsuarioID, nil
}

func pathID(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, &pos.AppError{Code: "id_invalido", Message: "Identificador inválido", Status: http.StatusBadRequest}
	}
	return id, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &pos.AppError{Code: "json_invalido", Message: "Datos inválidos: " + err.Error(), Status: http.StatusBadRequest}
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return &pos.AppError{Code: "json_invalido", Message: "Datos inválidos: se esperaba un único objeto JSON", Status: http.StatusBadRequest}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	if appErr, ok := pos.AsAppError(err); ok {
		writeJSON(w, appErr.Status, map[string]any{
			"error": map[string]string{"code": appErr.Code, "message": appErr.Message},
		})
		return
	}
	log.Printf("internal API error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": map[string]string{"code": "error_interno", "message": "Error interno del servidor"},
	})
}
