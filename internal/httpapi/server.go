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
	caja, err := s.svc.CloseCaja(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caja)
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
