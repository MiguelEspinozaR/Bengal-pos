package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// T-08/RT-01: cost amounts and recipe quantities are integers at the JSON
// boundary; decimals are rejected before reaching the service.
func TestT08_DecimalNumbersRejectedAtBoundary(t *testing.T) {
	handler := newTestServer(t, nil)
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/productos/1/costos",
			`{"usuario_id":1,"costo_bs":700.5,"fecha":"2026-10-01"}`},
		{http.MethodPut, "/api/productos/1/receta",
			`{"usuario_id":1,"items":[{"ingrediente_id":2,"cantidad":1.5}]}`},
		{http.MethodPut, "/api/productos/1/receta",
			`{"usuario_id":1,"rendimiento":1.5,"items":[]}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s: status = %d, want 400 (%s)",
				tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"code":"json_invalido"`) {
			t.Fatalf("%s %s: unexpected body %s", tc.method, tc.path, rec.Body.String())
		}
	}
}

// T-08: the new single-resource paths are mounted. An invalid path id is
// rejected before any service call, so this works with a nil service.
func TestT08_RoutesMounted(t *testing.T) {
	handler := newTestServer(t, nil)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/productos/abc/costos"},
		{http.MethodPost, "/api/productos/abc/costos"},
		{http.MethodGet, "/api/productos/abc/receta"},
		{http.MethodPut, "/api/productos/abc/receta"},
		{http.MethodGet, "/api/productos/abc/costo-margen"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s: status = %d, want 400 (route not mounted?)",
				tc.method, tc.path, rec.Code)
		}
	}
}
