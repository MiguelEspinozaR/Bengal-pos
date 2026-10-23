package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bengalpos/internal/pos"
)

// RF-18b: tips are not part of the API contract. Unknown fields are rejected
// before the use case runs, so a nil service is safe here and proves the tip
// never reaches persistence.
func TestCreateSaleRejectsTipFields(t *testing.T) {
	handler := NewServer(nil)
	for _, body := range []string{
		`{"usuario_id":1,"items":[{"producto_id":1,"cantidad":1}],"propina_bs":500}`,
		`{"usuario_id":1,"items":[{"producto_id":1,"cantidad":1}],"propina":500}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/ventas", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
		if !strings.Contains(rec.Body.String(), `"code":"json_invalido"`) {
			t.Fatalf("body %s: unexpected response %s", body, rec.Body.String())
		}
	}
}

// AC4: the browser receives a print-ready document plus the audit/copy
// metadata, without any hardware driver.
func TestWritePrintDocumentIsBrowserPrintable(t *testing.T) {
	rec := httptest.NewRecorder()
	writePrintDocument(rec, &pos.PrintDocument{
		Formato:   "html",
		Contenido: "<!doctype html><html><body>*** COPIA (2) ***</body></html>",
		Impresion: pos.Impresion{ID: 9, NumeroCopia: 2, Copia: true},
	})

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("X-Impresion-Id"); got != "9" {
		t.Fatalf("X-Impresion-Id = %q", got)
	}
	if got := rec.Header().Get("X-Numero-Copia"); got != "2" {
		t.Fatalf("X-Numero-Copia = %q", got)
	}
	if got := rec.Header().Get("X-Copia"); got != "true" {
		t.Fatalf("X-Copia = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "COPIA") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestWritePrintDocumentPlainText(t *testing.T) {
	rec := httptest.NewRecorder()
	writePrintDocument(rec, &pos.PrintDocument{
		Formato:   "texto",
		Contenido: "COMANDAS",
		Impresion: pos.Impresion{ID: 1, NumeroCopia: 1},
	})
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("X-Copia"); got != "false" {
		t.Fatalf("X-Copia = %q", got)
	}
}
