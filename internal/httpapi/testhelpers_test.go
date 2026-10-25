package httpapi

import (
	"context"
	"net/http"
	"testing"

	"bengalpos/internal/pos"
)

// newTestServer builds the transport with an injected authenticated session so
// boundary/validation tests can run without a database. Production NewServer
// always resolves sessions through the pos service.
func newTestServer(t *testing.T, svc *pos.Service) http.Handler {
	t.Helper()
	return newServer(svc, func(context.Context, string) (*pos.UsuarioSesion, error) {
		return &pos.UsuarioSesion{ID: 1, NombreCompleto: "Test Admin", Login: "admin", Rol: "administrador"}, nil
	})
}
