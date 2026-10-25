package pos

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const t09Password = "Clave1234"

func setupT09(t *testing.T) (*Service, *pgxpool.Pool, func()) {
	t.Helper()
	return setupT03(t)
}

func seedT09Usuario(t *testing.T, pool *pgxpool.Pool, nombre, login, password, rol string, activo bool) int64 {
	t.Helper()
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO usuarios (nombre_completo, login, password_hash, rol, activo)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		nombre, login, hash, rol, activo,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func countT09(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var total int64
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

func sessionCountT09(t *testing.T, pool *pgxpool.Pool, usuarioID int64, active bool) int64 {
	t.Helper()
	where := " AND revocada_en IS NULL AND expira_en > now()"
	if !active {
		where = ""
	}
	return countT09(t, pool,
		`SELECT COUNT(*) FROM sesiones_usuario WHERE usuario_id = $1`+where, usuarioID)
}

// AC1/AC7/AC8: login, logout, opaque sessions, bcrypt storage and bootstrap.
func TestT09_AC1_AC7_AC8_LoginSessionsBootstrap(t *testing.T) {
	svc, pool, cleanup := setupT09(t)
	defer cleanup()
	ctx := context.Background()

	// Empty database and missing env must fail clearly without leaking values.
	if err := svc.BootstrapOwner(ctx, BootstrapConfig{}); !errors.Is(err, ErrBootstrapRequerido) {
		t.Fatalf("bootstrap without config = %v, want ErrBootstrapRequerido", err)
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM usuarios`) != 0 {
		t.Fatal("bootstrap without config created a user")
	}

	cfg := BootstrapConfig{Login: " Dueno ", Password: "Dueno1234", Nombre: " Dueño Principal "}
	if err := svc.BootstrapOwner(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	var duenoID int64
	var duenoLogin, duenoRol string
	var activo bool
	var hash string
	if err := pool.QueryRow(ctx,
		`SELECT id, login, rol, activo, password_hash FROM usuarios`,
	).Scan(&duenoID, &duenoLogin, &duenoRol, &activo, &hash); err != nil {
		t.Fatal(err)
	}
	if duenoLogin != "dueno" || duenoRol != "dueno" || !activo {
		t.Fatalf("bootstrap user = login %q rol %q activo %v", duenoLogin, duenoRol, activo)
	}
	if hash == cfg.Password || !strings.HasPrefix(hash, "$2") {
		t.Fatal("bootstrap password is not stored as bcrypt")
	}
	if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost != 12 {
		t.Fatalf("bootstrap bcrypt cost = %d, err %v", cost, err)
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM auditoria WHERE accion = 'usuario_creado'`) < 1 {
		t.Fatal("bootstrap was not audited")
	}

	// Restarting with different env must not duplicate or modify users.
	if err := svc.BootstrapOwner(ctx, BootstrapConfig{
		Login: "otro", Password: "Otra1234", Nombre: "Otro",
	}); err != nil {
		t.Fatal(err)
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM usuarios`) != 1 {
		t.Fatal("bootstrap duplicated an existing user")
	}
	// Missing env is ignored once users exist (normal restarts).
	if err := svc.BootstrapOwner(ctx, BootstrapConfig{}); err != nil {
		t.Fatalf("bootstrap with existing users and no env = %v", err)
	}

	for _, login := range []string{"dueno", "NO_EXISTE"} {
		if _, err := svc.Login(ctx, login, "incorrecta1"); err != ErrCredencialesInvalidas {
			t.Fatalf("login(%q) with bad password = %v, want Credenciales inválidas", login, err)
		}
	}

	// Login normalizes the login but not the password.
	res, err := svc.Login(ctx, "  DUENO  ", cfg.Password)
	if err != nil {
		t.Fatal(err)
	}
	if res.Usuario.ID != duenoID || res.Usuario.Login != "dueno" || res.Token == "" {
		t.Fatalf("unexpected login result: %+v", res)
	}
	if countT09(t, pool,
		`SELECT COUNT(*) FROM sesiones_usuario WHERE token_hash = $1`, tokenHashBytes(res.Token),
	) != 1 {
		t.Fatal("session was not stored hashed")
	}
	if countT09(t, pool,
		`SELECT COUNT(*) FROM sesiones_usuario WHERE encode(token_hash, 'hex') = $1`, res.Token,
	) != 0 {
		t.Fatal("raw token was stored instead of its hash")
	}

	u, err := svc.ValidateSession(ctx, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != duenoID {
		t.Fatalf("validated user = %d, want %d", u.ID, duenoID)
	}
	if _, err := svc.ValidateSession(ctx, "not-a-session"); err != ErrNoAutenticado {
		t.Fatalf("invalid token error = %v", err)
	}

	// Absolute expiration: moving expira_en to the past invalidates it.
	if _, err := pool.Exec(ctx,
		`UPDATE sesiones_usuario SET expira_en = now() - interval '1 hour'
		  WHERE token_hash = $1`, tokenHashBytes(res.Token),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(ctx, res.Token); err != ErrNoAutenticado {
		t.Fatalf("expired token error = %v", err)
	}

	// Logout revokes; a second logout is idempotent.
	res2, err := svc.Login(ctx, "dueno", cfg.Password)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(ctx, res2.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(ctx, res2.Token); err != ErrNoAutenticado {
		t.Fatalf("revoked token still valid: %v", err)
	}
	if err := svc.Logout(ctx, res2.Token); err != nil {
		t.Fatalf("idempotent logout: %v", err)
	}
	if err := svc.Logout(ctx, ""); err != nil {
		t.Fatalf("logout without token: %v", err)
	}
}

// AC1/US-01: inactive users get exactly the same credentials error.
func TestT09_AC1_InactiveUserCannotLogin(t *testing.T) {
	svc, pool, cleanup := setupT09(t)
	defer cleanup()
	ctx := context.Background()

	inactiveID := seedT09Usuario(t, pool, "Inactivo", "inactivo", t09Password, "empleado", false)
	activeID := seedT09Usuario(t, pool, "Activo", "activo", t09Password, "empleado", true)

	if _, err := svc.Login(ctx, "inactivo", t09Password); err != ErrCredencialesInvalidas {
		t.Fatalf("inactive login = %v, want Credenciales inválidas", err)
	}
	if _, err := svc.Login(ctx, "activo", "otra-clave1"); err != ErrCredencialesInvalidas {
		t.Fatalf("wrong password login = %v, want Credenciales inválidas", err)
	}
	if _, err := svc.Login(ctx, "no-existe", t09Password); err != ErrCredencialesInvalidas {
		t.Fatalf("unknown login = %v, want Credenciales inválidas", err)
	}
	if _, err := svc.Login(ctx, "activo", t09Password); err != nil {
		t.Fatalf("active login failed: %v", err)
	}
	if sessionCountT09(t, pool, inactiveID, true) != 0 {
		t.Fatal("inactive user received a session")
	}
	if sessionCountT09(t, pool, activeID, true) != 1 {
		t.Fatal("active user did not receive a session")
	}
}

// AC3/RF-09..RF-11: role matrix, unique login, normalization and password
// policy. Rejected requests persist nothing.
func TestT09_AC3_CreateUsersRoleMatrix(t *testing.T) {
	svc, pool, cleanup := setupT09(t)
	defer cleanup()
	ctx := context.Background()

	duenoID := seedT09Usuario(t, pool, "Dueño", "dueno9", t09Password, "dueno", true)
	adminID := seedT09Usuario(t, pool, "Admin", "admin9", t09Password, "administrador", true)
	encID := seedT09Usuario(t, pool, "Encargado", "enc9", t09Password, "encargado", true)
	empID := seedT09Usuario(t, pool, "Empleado", "emp9", t09Password, "empleado", true)

	create := func(actor int64, login, rol string) (*Usuario, error) {
		return svc.CreateUsuario(ctx, CreateUsuarioInput{
			UsuarioID: actor, NombreCompleto: "Nuevo " + login, Login: login,
			Password: t09Password, Rol: rol,
		})
	}

	// Dueño can create administrador/encargado/empleado.
	for _, rol := range []string{"administrador", "encargado", "empleado"} {
		if _, err := create(duenoID, "dueno-crea-"+rol, rol); err != nil {
			t.Fatalf("dueño create %s: %v", rol, err)
		}
	}
	// Administrator can create encargado/empleado only.
	for _, rol := range []string{"encargado", "empleado"} {
		if _, err := create(adminID, "admin-crea-"+rol, rol); err != nil {
			t.Fatalf("admin create %s: %v", rol, err)
		}
	}
	for _, tc := range []struct {
		actor int64
		rol   string
	}{
		{adminID, "administrador"},
		{adminID, "dueno"},
		{duenoID, "dueno"},
		{encID, "empleado"},
		{empID, "empleado"},
	} {
		if _, err := create(tc.actor, "prohibido", tc.rol); err == nil {
			t.Fatalf("actor %d created forbidden role %s", tc.actor, tc.rol)
		}
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM usuarios WHERE login = 'prohibido'`) != 0 {
		t.Fatal("forbidden role persisted a user")
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM auditoria WHERE accion = 'usuario_creado'`) < 3 {
		t.Fatal("user creations were not audited")
	}

	// Duplicate login is 409 and persists nothing.
	before := countT09(t, pool, `SELECT COUNT(*) FROM usuarios`)
	if _, err := create(duenoID, " ADMIN9 ", "empleado"); err != ErrLoginDuplicado {
		t.Fatalf("duplicate login = %v, want El login ya existe", err)
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM usuarios`) != before {
		t.Fatal("duplicate login persisted a user")
	}

	// Login is trimmed/lowercased; password is not trimmed.
	created, err := create(duenoID, "  Maria.Lopez  ", "empleado")
	if err != nil {
		t.Fatal(err)
	}
	if created.Login != "maria.lopez" {
		t.Fatalf("stored login = %q", created.Login)
	}
	if _, err := create(duenoID, "MARIA.LOPEZ", "empleado"); err != ErrLoginDuplicado {
		t.Fatalf("normalized duplicate = %v", err)
	}
	spaced, err := svc.CreateUsuario(ctx, CreateUsuarioInput{
		UsuarioID: duenoID, NombreCompleto: "Espacios", Login: "espacios",
		Password: " Passw0rd ", Rol: "empleado",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spaced.ID == 0 {
		t.Fatal("spaced password user not created")
	}
	if _, err := svc.Login(ctx, "espacios", "Passw0rd"); err != ErrCredencialesInvalidas {
		t.Fatalf("trimmed password login = %v", err)
	}
	if _, err := svc.Login(ctx, "espacios", " Passw0rd "); err != nil {
		t.Fatalf("untrimmed password login failed: %v", err)
	}

	// Invalid role and password policy are 422 and persist nothing.
	before = countT09(t, pool, `SELECT COUNT(*) FROM usuarios`)
	if _, err := create(duenoID, "rol-vacio", ""); err != ErrRolInvalido {
		t.Fatalf("empty role = %v", err)
	}
	if _, err := create(duenoID, "rol-malo", "supervisor"); err != ErrRolInvalido {
		t.Fatalf("invalid role = %v", err)
	}
	for _, password := range []string{"short1", "abcdefgh", "12345678", strings.Repeat("a", 72) + "1"} {
		if _, err := svc.CreateUsuario(ctx, CreateUsuarioInput{
			UsuarioID: duenoID, NombreCompleto: "Politica", Login: "politica",
			Password: password, Rol: "empleado",
		}); err == nil {
			t.Fatalf("weak password %q accepted", password)
		}
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM usuarios`) != before {
		t.Fatal("invalid role/password persisted a user")
	}
}

// AC4/AC5/AC7: listing, logical deactivation, reactivation, role immutability,
// session revocation and audit; history keeps the user id.
func TestT09_AC4_AC5_AC7_AdministrationAndHistory(t *testing.T) {
	svc, pool, cleanup := setupT09(t)
	defer cleanup()
	ctx := context.Background()

	duenoID := seedT09Usuario(t, pool, "Dueño", "dueno9", t09Password, "dueno", true)
	adminID := seedT09Usuario(t, pool, "Admin", "admin9", t09Password, "administrador", true)
	admin2ID := seedT09Usuario(t, pool, "Admin Dos", "admin29", t09Password, "administrador", true)
	encID := seedT09Usuario(t, pool, "Encargado", "enc9", t09Password, "encargado", true)
	empID := seedT09Usuario(t, pool, "Empleado", "emp9", t09Password, "empleado", true)

	// A sale created by the employee must keep its user id after deactivation.
	var productoID, depositoID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, precio_venta_bs, stock_minimo)
		 VALUES ('T09-P1', 'Producto T09', 1000, 0) RETURNING id`,
	).Scan(&productoID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO depositos (nombre) VALUES ('T09 Principal') RETURNING id`,
	).Scan(&depositoID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO stock_depositos (producto_id, deposito_id, cantidad) VALUES ($1, $2, 10)`,
		productoID, depositoID,
	); err != nil {
		t.Fatal(err)
	}
	sale, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: empID,
		Items:     []CreateSaleItem{{ProductoID: productoID, Cantidad: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	empLogin, err := svc.Login(ctx, "emp9", t09Password)
	if err != nil {
		t.Fatal(err)
	}

	all, err := svc.ListUsuarios(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("users = %d, want 5", len(all))
	}
	for _, u := range all {
		if u.Login == "" || u.ID == 0 {
			t.Fatalf("unexpected listing row: %+v", u)
		}
	}

	// Name change does not change the hash.
	var hashBefore string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM usuarios WHERE id = $1`, empID).Scan(&hashBefore); err != nil {
		t.Fatal(err)
	}
	newName := "Empleado Renombrado"
	updated, err := svc.UpdateUsuario(ctx, empID, UpdateUsuarioInput{UsuarioID: adminID, NombreCompleto: &newName})
	if err != nil {
		t.Fatal(err)
	}
	if updated.NombreCompleto != newName {
		t.Fatalf("name = %q", updated.NombreCompleto)
	}
	var hashAfter string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM usuarios WHERE id = $1`, empID).Scan(&hashAfter); err != nil {
		t.Fatal(err)
	}
	if hashBefore != hashAfter {
		t.Fatal("name change altered the password hash")
	}

	// Role is immutable.
	rol := "administrador"
	if _, err := svc.UpdateUsuario(ctx, empID, UpdateUsuarioInput{UsuarioID: adminID, Rol: &rol}); err != ErrRolInmutable {
		t.Fatalf("role change = %v, want rol inmutable", err)
	}

	// Administrator cannot manage another administrator or the owner.
	inactive := false
	if _, err := svc.UpdateUsuario(ctx, admin2ID, UpdateUsuarioInput{UsuarioID: adminID, Activo: &inactive}); err != ErrNoAutorizado {
		t.Fatalf("admin modifying admin = %v", err)
	}
	if _, err := svc.UpdateUsuario(ctx, duenoID, UpdateUsuarioInput{UsuarioID: adminID, Activo: &inactive}); err != ErrNoAutorizado {
		t.Fatalf("admin modifying owner = %v", err)
	}
	// Owner can manage administrators.
	newAdminName := "Admin Renombrado"
	if _, err := svc.UpdateUsuario(ctx, adminID, UpdateUsuarioInput{UsuarioID: duenoID, NombreCompleto: &newAdminName}); err != nil {
		t.Fatalf("owner modifying admin: %v", err)
	}

	// Nobody can deactivate themselves.
	if _, err := svc.UpdateUsuario(ctx, adminID, UpdateUsuarioInput{UsuarioID: adminID, Activo: &inactive}); err != ErrAutoDesactivacion {
		t.Fatalf("self deactivation = %v", err)
	}

	// Deactivate the employee: sessions revoked in the same operation, login
	// rejected, still present with incluir_inactivos and history intact.
	if _, err := svc.UpdateUsuario(ctx, empID, UpdateUsuarioInput{UsuarioID: adminID, Activo: &inactive}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(ctx, empLogin.Token); err != ErrNoAutenticado {
		t.Fatalf("session after deactivation = %v", err)
	}
	if _, err := svc.Login(ctx, "emp9", t09Password); err != ErrCredencialesInvalidas {
		t.Fatalf("inactive login = %v", err)
	}
	activeOnly, err := svc.ListUsuarios(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range activeOnly {
		if u.ID == empID {
			t.Fatal("inactive user appears in the default listing")
		}
	}
	all, err = svc.ListUsuarios(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	foundInactive := false
	for _, u := range all {
		if u.ID == empID && !u.Activo {
			foundInactive = true
		}
	}
	if !foundInactive {
		t.Fatal("inactive user missing from incluir_inactivos listing")
	}
	var historyUser int64
	if err := pool.QueryRow(ctx, `SELECT usuario_id FROM ventas WHERE id = $1`, sale.ID).Scan(&historyUser); err != nil {
		t.Fatal(err)
	}
	if historyUser != empID {
		t.Fatalf("sale user = %d, want %d", historyUser, empID)
	}
	if countT09(t, pool,
		`SELECT COUNT(*) FROM auditoria WHERE accion = 'usuario_desactivado' AND entidad_id = $1 AND usuario_id = $2`,
		empID, adminID,
	) != 1 {
		t.Fatal("deactivation not audited with actor and target")
	}

	// Reactivation allows a new login but does not restore old sessions.
	active := true
	if _, err := svc.UpdateUsuario(ctx, empID, UpdateUsuarioInput{UsuarioID: adminID, Activo: &active}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(ctx, empLogin.Token); err != ErrNoAutenticado {
		t.Fatal("revoked session became valid after reactivation")
	}
	if _, err := svc.Login(ctx, "emp9", t09Password); err != nil {
		t.Fatalf("login after reactivation: %v", err)
	}
	if countT09(t, pool,
		`SELECT COUNT(*) FROM auditoria WHERE accion = 'usuario_activado' AND entidad_id = $1`, empID,
	) != 1 {
		t.Fatal("reactivation not audited")
	}

	// Password reset rotates the hash and revokes sessions.
	empSession, err := svc.Login(ctx, "emp9", t09Password)
	if err != nil {
		t.Fatal(err)
	}
	var hashEmployee string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM usuarios WHERE id = $1`, empID).Scan(&hashEmployee); err != nil {
		t.Fatal(err)
	}
	newPassword := "NuevaClave99"
	if _, err := svc.UpdateUsuario(ctx, empID, UpdateUsuarioInput{UsuarioID: adminID, Password: &newPassword}); err != nil {
		t.Fatal(err)
	}
	var newHash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM usuarios WHERE id = $1`, empID).Scan(&newHash); err != nil {
		t.Fatal(err)
	}
	if newHash == hashEmployee {
		t.Fatal("password reset did not change the hash")
	}
	if _, err := svc.ValidateSession(ctx, empSession.Token); err != ErrNoAutenticado {
		t.Fatalf("session after reset = %v", err)
	}
	if _, err := svc.Login(ctx, "emp9", t09Password); err != ErrCredencialesInvalidas {
		t.Fatal("old password still works after reset")
	}
	if _, err := svc.Login(ctx, "emp9", newPassword); err != nil {
		t.Fatalf("new password login: %v", err)
	}
	if countT09(t, pool,
		`SELECT COUNT(*) FROM auditoria WHERE accion = 'usuario_reset_password' AND entidad_id = $1 AND usuario_id = $2`,
		empID, adminID,
	) != 1 {
		t.Fatal("password reset not audited")
	}

	_ = encID
}

// AC6/RT-05: the discount authorizer must be an active admin/dueño; a
// deactivated authorizer is rejected and nothing is persisted.
func TestT09_AC6_DiscountAuthorizerMustBeActiveAdmin(t *testing.T) {
	svc, pool, cleanup := setupT09(t)
	defer cleanup()
	ctx := context.Background()

	duenoID := seedT09Usuario(t, pool, "Dueño", "dueno9", t09Password, "dueno", true)
	adminID := seedT09Usuario(t, pool, "Admin", "admin9", t09Password, "administrador", true)
	empID := seedT09Usuario(t, pool, "Empleado", "emp9", t09Password, "empleado", true)

	var productoID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO productos (codigo, nombre, precio_venta_bs, stock_minimo)
		 VALUES ('T09-P2', 'Producto T09', 1000, 0) RETURNING id`,
	).Scan(&productoID); err != nil {
		t.Fatal(err)
	}

	// Employee cannot authorize a discount.
	if _, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: empID, Items: []CreateSaleItem{{ProductoID: productoID, Cantidad: 1}},
		DescuentoBs: 100, DescuentoUsuarioID: &empID,
	}); err != ErrNoAutorizado {
		t.Fatalf("employee discount authorizer = %v", err)
	}

	// Admin authorizes while active.
	if _, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: empID, Items: []CreateSaleItem{{ProductoID: productoID, Cantidad: 1}},
		DescuentoBs: 100, DescuentoUsuarioID: &adminID,
	}); err != nil {
		t.Fatalf("active admin authorizer: %v", err)
	}

	// Deactivated admin can no longer authorize and the sale is not persisted.
	inactive := false
	if _, err := svc.UpdateUsuario(ctx, adminID, UpdateUsuarioInput{UsuarioID: duenoID, Activo: &inactive}); err != nil {
		t.Fatal(err)
	}
	before := countT09(t, pool, `SELECT COUNT(*) FROM ventas`)
	if _, err := svc.CreateSale(ctx, CreateSaleInput{
		UsuarioID: empID, Items: []CreateSaleItem{{ProductoID: productoID, Cantidad: 1}},
		DescuentoBs: 100, DescuentoUsuarioID: &adminID,
	}); err != ErrNoAutorizado {
		t.Fatalf("inactive admin authorizer = %v", err)
	}
	if countT09(t, pool, `SELECT COUNT(*) FROM ventas`) != before {
		t.Fatal("rejected discount persisted a sale")
	}
}
