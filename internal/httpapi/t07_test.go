package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// AC1/AC7/AC9: decimal quantities or amounts never reach the service. The
// decoder rejects them before execution, which is safe with a nil service.
func TestT07_DecimalNumbersRejectedAtBoundary(t *testing.T) {
	handler := newTestServer(t, nil)
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/productos",
			`{"usuario_id":1,"codigo":"TOR-001","nombre":"Torta","precio_venta_bs":1000.5}`},
		{http.MethodPost, "/api/inventario/movimientos",
			`{"usuario_id":1,"producto_id":1,"deposito_id":1,"tipo":"entrada","cantidad":1.5}`},
		{http.MethodPost, "/api/inventario/traslados",
			`{"usuario_id":1,"producto_id":1,"deposito_origen_id":1,"deposito_destino_id":2,"cantidad":2.5}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s: status = %d, want 400 (%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"code":"json_invalido"`) {
			t.Fatalf("%s %s: unexpected body %s", tc.method, tc.path, rec.Body.String())
		}
	}
}

// Query filters must be positive integers before hitting the database.
func TestT07_StockQueryValidation(t *testing.T) {
	handler := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/inventario/stock?producto_id=abc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"parametro_invalido"`) {
		t.Fatalf("unexpected body %s", rec.Body.String())
	}
}
