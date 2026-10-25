package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"bengalpos/internal/database"
	"bengalpos/internal/pos"
	"bengalpos/internal/testdb"
	"bengalpos/migrations"
)

const t09HTTPPassword = "Clave1234"

func setupT09HTTP(t *testing.T) (*pos.Service, *pgxpool.Pool, http.Handler, func()) {
	t.Helper()
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	dsn := testdb.DSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "t09h_" + hex.EncodeToString(b)

	adminPool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	schemaDSN := dsn
	if strings.Contains(schemaDSN, "?") {
		schemaDSN += "&search_path=" + schema + ",public"
	} else {
		schemaDSN += "?search_path=" + schema
	}
	pool, err := database.NewPool(ctx, schemaDSN)
	if err != nil {
		adminPool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
		adminPool.Close()
		t.Fatal(err)
	}
	if _, err := database.ApplyMigrations(ctx, pool, migrations.FS); err != nil {
		pool.Close()
		adminPool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
		adminPool.Close()
		t.Fatal(err)
	}
	svc := pos.NewServiceWithConfig(pool, pos.Config{SessionTTL: 12 * time.Hour})
	handler := NewServer(svc)
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pool.Close()
		adminPool.Exec(ctx, fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
		adminPool.Close()
	}
	return svc, pool, handler, cleanup
}

func t09Bootstrap(t *testing.T, svc *pos.Service) int64 {
	t.Helper()
	if err := svc.BootstrapOwner(context.Background(), pos.BootstrapConfig{
		Login: "dueno", Password: t09HTTPPassword, Nombre: "Dueño Principal",
	}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Login(context.Background(), "dueno", t09HTTPPassword)
	if err != nil {
		t.Fatal(err)
	}
	return res.Usuario.ID
}

func t09Request(t *testing.T, h http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func t09LoginHTTP(t *testing.T, h http.Handler, login, password string) (*http.Cookie, pos.UsuarioSesion) {
	t.Helper()
	rec := t09Request(t, h, http.MethodPost, "/api/auth/login", map[string]string{
		"login": login, "password": password,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %q: status = %d, body = %s", login, rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "bengalpos_sesion" {
		t.Fatalf("login cookies = %+v", cookies)
	}
	var out struct {
		Usuario pos.UsuarioSesion `json:"usuario"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Usuario.ID == 0 || out.Usuario.Login == "" || out.Usuario.Rol == "" {
		t.Fatalf("login user = %+v", out.Usuario)
	}
	return cookies[0], out.Usuario
}

func t09Error(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var out struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return out.Error.Code, out.Error.Message
}

// AC1/AC2/AC7: cookie session lifecycle, public health, exact 401 messages and
// no secret leakage in responses.
func TestT09_HTTP_AC1_SessionLifecycle(t *testing.T) {
	svc, pool, handler, cleanup := setupT09HTTP(t)
	defer cleanup()
	t09Bootstrap(t, svc)

	// healthz remains public.
	if rec := t09Request(t, handler, http.MethodGet, "/healthz", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}

	// Protected endpoint without cookie is 401 and does not execute.
	for _, path := range []string{"/api/auth/me", "/api/productos", "/api/usuarios"} {
		rec := t09Request(t, handler, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without cookie = %d (%s)", path, rec.Code, rec.Body.String())
		}
		code, message := t09Error(t, rec)
		if code != "no_autenticado" || message != "Debe iniciar sesión" {
			t.Fatalf("%s error = (%s) %q", path, code, message)
		}
	}

	// Wrong password, unknown login and inactive user all return the same 401.
	inactiveID := 0
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM usuarios WHERE login = 'dueno'`).Scan(&inactiveID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ login, password string }{
		{"dueno", "incorrecta1"},
		{"no-existe", t09HTTPPassword},
	} {
		rec := t09Request(t, handler, http.MethodPost, "/api/auth/login", map[string]string{
			"login": tc.login, "password": tc.password,
		}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("login %q = %d, want 401", tc.login, rec.Code)
		}
		_, message := t09Error(t, rec)
		if message != "Credenciales inválidas" {
			t.Fatalf("login error = %q", message)
		}
		if strings.Contains(rec.Body.String(), tc.password) {
			t.Fatal("login response leaked the password")
		}
	}

	cookie, user := t09LoginHTTP(t, handler, "  DUENO  ", t09HTTPPassword)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode ||
		cookie.Path != "/" || cookie.MaxAge != int((12*time.Hour).Seconds()) || cookie.Secure {
		t.Fatalf("cookie attributes = %+v", cookie)
	}
	if cookie.Value == "" || len(cookie.Value) < 32 {
		t.Fatalf("cookie token looks weak: %q", cookie.Value)
	}
	if user.Login != "dueno" || user.Rol != "dueno" {
		t.Fatalf("login user = %+v", user)
	}

	// me returns the same identity; the response never contains the token.
	rec := t09Request(t, handler, http.MethodGet, "/api/auth/me", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d (%s)", rec.Code, rec.Body.String())
	}
	var me struct {
		Usuario pos.UsuarioSesion `json:"usuario"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Usuario.ID != user.ID || me.Usuario.Login != "dueno" {
		t.Fatalf("me = %+v", me.Usuario)
	}
	if strings.Contains(rec.Body.String(), cookie.Value) || strings.Contains(rec.Body.String(), "password_hash") {
		t.Fatal("me response leaked a secret")
	}

	// Invalid and expired cookies are rejected.
	bad := &http.Cookie{Name: "bengalpos_sesion", Value: "invalida"}
	if rec := t09Request(t, handler, http.MethodGet, "/api/auth/me", nil, bad); rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid cookie = %d", rec.Code)
	}
	expiredCookie, _ := t09LoginHTTP(t, handler, "dueno", t09HTTPPassword)
	if _, err := pool.Exec(context.Background(),
		`UPDATE sesiones_usuario SET expira_en = now() - interval '1 hour'
		  WHERE encode(token_hash, 'hex') <> '' AND usuario_id = $1`,
		user.ID,
	); err != nil {
		t.Fatal(err)
	}
	if rec := t09Request(t, handler, http.MethodGet, "/api/auth/me", nil, expiredCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired cookie = %d", rec.Code)
	}

	// Logout revokes and clears the cookie; it is idempotent without session.
	freshCookie, _ := t09LoginHTTP(t, handler, "dueno", t09HTTPPassword)
	rec = t09Request(t, handler, http.MethodPost, "/api/auth/logout", nil, freshCookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d (%s)", rec.Code, rec.Body.String())
	}
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 || cleared[0].Name != "bengalpos_sesion" {
		t.Fatalf("logout cookie = %+v", cleared)
	}
	if rec := t09Request(t, handler, http.MethodGet, "/api/auth/me", nil, freshCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d", rec.Code)
	}
	if rec := t09Request(t, handler, http.MethodPost, "/api/auth/logout", nil, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("idempotent logout = %d", rec.Code)
	}

	// Secure is opt-in through SESSION_COOKIE_SECURE.
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	secureHandler := NewServer(svc)
	secureCookie, _ := t09LoginHTTP(t, secureHandler, "dueno", t09HTTPPassword)
	if !secureCookie.Secure {
		t.Fatal("SESSION_COOKIE_SECURE=true did not mark the cookie Secure")
	}
}

// AC2/RNF-02: the session, not body or query usuario_id, is the identity.
func TestT09_HTTP_AC2_NoImpersonation(t *testing.T) {
	svc, pool, handler, cleanup := setupT09HTTP(t)
	defer cleanup()
	duenoID := t09Bootstrap(t, svc)
	ctx := context.Background()

	admin, err := svc.CreateUsuario(ctx, pos.CreateUsuarioInput{
		UsuarioID: duenoID, NombreCompleto: "Admin", Login: "admin9",
		Password: t09HTTPPassword, Rol: "administrador",
	})
	if err != nil {
		t.Fatal(err)
	}
	employee, err := svc.CreateUsuario(ctx, pos.CreateUsuarioInput{
		UsuarioID: duenoID, NombreCompleto: "Empleado", Login: "emp9",
		Password: t09HTTPPassword, Rol: "empleado",
	})
	if err != nil {
		t.Fatal(err)
	}
	var productoID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, precio_venta_bs, stock_minimo)
		 VALUES ('T09-H1', 'Producto HTTP', 1000, 0) RETURNING id`,
	).Scan(&productoID); err != nil {
		t.Fatal(err)
	}

	empCookie, _ := t09LoginHTTP(t, handler, "emp9", t09HTTPPassword)
	rec := t09Request(t, handler, http.MethodPost, "/api/ventas", map[string]any{
		"usuario_id": admin.ID,
		"items":      []map[string]any{{"producto_id": productoID, "cantidad": 1}},
	}, empCookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create sale = %d (%s)", rec.Code, rec.Body.String())
	}
	var sale pos.Sale
	if err := json.Unmarshal(rec.Body.Bytes(), &sale); err != nil {
		t.Fatal(err)
	}
	if sale.UsuarioID != employee.ID {
		t.Fatalf("sale user = %d, want session user %d", sale.UsuarioID, employee.ID)
	}

	// An employee cannot use an administrator id in the body to gain role.
	rec = t09Request(t, handler, http.MethodPost, "/api/productos", map[string]any{
		"usuario_id":      admin.ID,
		"codigo":          "HACK-1",
		"nombre":          "No permitido",
		"precio_venta_bs": 100,
	}, empCookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("employee product creation = %d (%s)", rec.Code, rec.Body.String())
	}
	var count int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM productos WHERE codigo = 'HACK-1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("impersonated body usuario_id persisted a product")
	}
}

// AC3/AC4/RF-09..RF-12: HTTP user administration, role matrix, logical
// deactivation and password reset with session revocation.
func TestT09_HTTP_AC3_AC4_Users(t *testing.T) {
	svc, pool, handler, cleanup := setupT09HTTP(t)
	defer cleanup()
	t09Bootstrap(t, svc)
	ownerCookie, owner := t09LoginHTTP(t, handler, "dueno", t09HTTPPassword)

	createUser := func(cookie *http.Cookie, login, rol string) *httptest.ResponseRecorder {
		return t09Request(t, handler, http.MethodPost, "/api/usuarios", map[string]string{
			"nombre_completo": "Usuario " + login,
			"login":           login,
			"password":        t09HTTPPassword,
			"rol":             rol,
		}, cookie)
	}
	decodeUser := func(rec *httptest.ResponseRecorder) pos.Usuario {
		var u pos.Usuario
		if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
			t.Fatalf("decode user %q: %v", rec.Body.String(), err)
		}
		return u
	}

	// Dueño creates an administrator.
	rec := createUser(ownerCookie, "admin-http", "administrador")
	if rec.Code != http.StatusCreated {
		t.Fatalf("dueño create admin = %d (%s)", rec.Code, rec.Body.String())
	}
	admin := decodeUser(rec)
	if admin.Rol != "administrador" || !admin.Activo {
		t.Fatalf("created admin = %+v", admin)
	}
	adminCookie, _ := t09LoginHTTP(t, handler, "admin-http", t09HTTPPassword)

	// Administrator creates encargado/empleado but not another administrator.
	rec = createUser(adminCookie, "enc-http", "encargado")
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create encargado = %d (%s)", rec.Code, rec.Body.String())
	}
	enc := decodeUser(rec)
	rec = createUser(adminCookie, "emp-http", "empleado")
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create empleado = %d (%s)", rec.Code, rec.Body.String())
	}
	emp := decodeUser(rec)
	rec = createUser(adminCookie, "admin2-http", "administrador")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin create admin = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = createUser(ownerCookie, "dueno2-http", "dueno")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("dueno create dueno = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := createUser(ownerCookie, "bad-role", "supervisor"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid role = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = createUser(ownerCookie, "admin-http", "empleado")
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate login = %d (%s)", rec.Code, rec.Body.String())
	}
	if _, message := t09Error(t, rec); message != "El login ya existe" {
		t.Fatalf("duplicate login message = %q", message)
	}

	// Employee and encargado cannot list or create users.
	empCookie, _ := t09LoginHTTP(t, handler, "emp-http", t09HTTPPassword)
	encCookie, _ := t09LoginHTTP(t, handler, "enc-http", t09HTTPPassword)
	for _, cookie := range []*http.Cookie{empCookie, encCookie} {
		if rec := t09Request(t, handler, http.MethodGet, "/api/usuarios", nil, cookie); rec.Code != http.StatusForbidden {
			t.Fatalf("non-admin list users = %d", rec.Code)
		}
		if rec := createUser(cookie, "prohibido-http", "empleado"); rec.Code != http.StatusForbidden {
			t.Fatalf("non-admin create user = %d (%s)", rec.Code, rec.Body.String())
		}
	}

	// Listing never exposes password_hash; default hides inactive.
	rec = t09Request(t, handler, http.MethodGet, "/api/usuarios", nil, adminCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "password_hash") || strings.Contains(rec.Body.String(), t09HTTPPassword) {
		t.Fatal("user listing leaked password data")
	}
	var list struct {
		Usuarios []pos.Usuario `json:"usuarios"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Usuarios) < 4 {
		t.Fatalf("default listing = %d users", len(list.Usuarios))
	}

	// Deactivation revokes the employee session in the same operation.
	rec = t09Request(t, handler, http.MethodPut, fmt.Sprintf("/api/usuarios/%d", emp.ID),
		map[string]any{"activo": false}, adminCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("deactivate employee = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := t09Request(t, handler, http.MethodGet, "/api/auth/me", nil, empCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after deactivation = %d", rec.Code)
	}
	rec = t09Request(t, handler, http.MethodPost, "/api/auth/login", map[string]string{
		"login": "emp-http", "password": t09HTTPPassword,
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("inactive login = %d", rec.Code)
	}
	rec = t09Request(t, handler, http.MethodGet, "/api/usuarios", nil, adminCookie)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, u := range list.Usuarios {
		if u.ID == emp.ID {
			t.Fatal("inactive user appears in the default listing")
		}
	}
	rec = t09Request(t, handler, http.MethodGet, "/api/usuarios?incluir_inactivos=true", nil, adminCookie)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range list.Usuarios {
		if u.ID == emp.ID && !u.Activo {
			found = true
		}
	}
	if !found {
		t.Fatal("inactive user missing with incluir_inactivos=true")
	}

	// No self-deactivation and no administrador modifying a dueño/administrador.
	if rec := t09Request(t, handler, http.MethodPut, fmt.Sprintf("/api/usuarios/%d", admin.ID),
		map[string]any{"activo": false}, adminCookie); rec.Code != http.StatusConflict {
		t.Fatalf("admin self deactivation = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := t09Request(t, handler, http.MethodPut, fmt.Sprintf("/api/usuarios/%d", owner.ID),
		map[string]any{"activo": false}, adminCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("admin modifying owner = %d (%s)", rec.Code, rec.Body.String())
	}

	// Dueño can edit an administrator; role is immutable.
	rec = t09Request(t, handler, http.MethodPut, fmt.Sprintf("/api/usuarios/%d", admin.ID),
		map[string]any{"nombre_completo": "Admin Editado"}, ownerCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner edit admin = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := t09Request(t, handler, http.MethodPut, fmt.Sprintf("/api/usuarios/%d", admin.ID),
		map[string]any{"rol": "empleado"}, ownerCookie); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("role change = %d (%s)", rec.Code, rec.Body.String())
	}

	// Password reset revokes the encargado session and rotates credentials.
	newPassword := "NuevaClave99"
	rec = t09Request(t, handler, http.MethodPut, fmt.Sprintf("/api/usuarios/%d", enc.ID),
		map[string]any{"password": newPassword}, adminCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset password = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := t09Request(t, handler, http.MethodGet, "/api/auth/me", nil, encCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after reset = %d", rec.Code)
	}
	rec = t09Request(t, handler, http.MethodPost, "/api/auth/login", map[string]string{
		"login": "enc-http", "password": t09HTTPPassword,
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password after reset = %d", rec.Code)
	}
	t09LoginHTTP(t, handler, "enc-http", newPassword)

	// Reactivation permits a new login.
	rec = t09Request(t, handler, http.MethodPut, fmt.Sprintf("/api/usuarios/%d", emp.ID),
		map[string]any{"activo": true}, adminCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("reactivate employee = %d (%s)", rec.Code, rec.Body.String())
	}
	empCookie2, _ := t09LoginHTTP(t, handler, "emp-http", t09HTTPPassword)
	if empCookie2.Value == empCookie.Value {
		t.Fatal("reactivation restored the revoked session token")
	}

	// The old revoked session stays invalid.
	if rec := t09Request(t, handler, http.MethodGet, "/api/auth/me", nil, empCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session after reactivation = %d", rec.Code)
	}

	// No physical delete endpoint.
	if rec := t09Request(t, handler, http.MethodDelete, fmt.Sprintf("/api/usuarios/%d", emp.ID), nil, ownerCookie); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE /api/usuarios = %d, want no route", rec.Code)
	}
	var rows int64
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM usuarios WHERE id = $1`, emp.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatal("deactivated user was physically removed")
	}
}
