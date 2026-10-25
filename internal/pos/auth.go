package pos

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// dummyHash is used when a login does not exist so that bcrypt comparison
// takes roughly the same time as a real check (constant-time-ish response).
var dummyHash []byte

func init() {
	// Precompute a valid bcrypt hash at cost 12 so timing cannot leak whether a
	// user exists.
	h, err := bcrypt.GenerateFromPassword([]byte("dummy-password-not-used-9f3e"), 12)
	if err != nil {
		panic(fmt.Sprintf("init dummy hash: %v", err))
	}
	dummyHash = h
}

const bcryptCost = 12

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func checkPassword(hash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func generateToken() (string, []byte, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	return token, sum[:], nil
}

func tokenHashBytes(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func normalizeLogin(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 72 {
		return ErrPasswordLongitud
	}
	var hasLetter bool
	var hasDigit bool
	for i := 0; i < len(password); i++ {
		c := password[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			hasLetter = true
			continue
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrPasswordDebil
	}
	return nil
}

func scanUsuarioSesion(row pgx.Row) (UsuarioSesion, error) {
	var u UsuarioSesion
	err := row.Scan(&u.ID, &u.NombreCompleto, &u.Login, &u.Rol)
	return u, err
}

// Login authenticates a user with an opaque server session. Invalid, inactive
// and non-existent users return the exact message "Credenciales inválidas"
// with no indication of which part failed. A dummy hash is compared when the
// login does not exist so response timing cannot be used to enumerate users.
func (s *Service) Login(ctx context.Context, login, password string) (*LoginResult, error) {
	l := normalizeLogin(login)
	if l == "" || strings.TrimSpace(password) == "" {
		return nil, ErrCredencialesInvalidas
	}

	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin login: %w", err)
	}
	defer rollback(ctx, tx)

	var hash string
	var activo bool
	var userID int64
	var nombre, userLogin, rol string
	err = tx.QueryRow(ctx,
		`SELECT id, nombre_completo, login, rol, password_hash, activo
		   FROM usuarios WHERE lower(login) = $1`,
		l,
	).Scan(&userID, &nombre, &userLogin, &rol, &hash, &activo)
	if errors.Is(err, pgx.ErrNoRows) {
		// Compare against a dummy hash to keep timing consistent.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		_ = tx.Rollback(ctx)
		return nil, ErrCredencialesInvalidas
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("load login user: %w", err)
	}
	if err := checkPassword(hash, password); err != nil || !activo {
		_ = tx.Rollback(ctx)
		return nil, ErrCredencialesInvalidas
	}

	token, tokenHash, err := generateToken()
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	expira := now.Add(s.SessionTTL())
	if _, err := tx.Exec(ctx,
		`INSERT INTO sesiones_usuario (token_hash, usuario_id, creado_en, expira_en)
		 VALUES ($1, $2, $3, $4)`,
		tokenHash, userID, now, expira,
	); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("create session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit login: %w", err)
	}

	return &LoginResult{
		Usuario: UsuarioSesion{
			ID:             userID,
			NombreCompleto: nombre,
			Login:          userLogin,
			Rol:            rol,
		},
		Token:    token,
		ExpiraEn: expira,
	}, nil
}

// Logout revokes the session identified by the raw token. It is idempotent
// when there is no session.
func (s *Service) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE sesiones_usuario
		    SET revocada_en = COALESCE(revocada_en, now())
		  WHERE token_hash = $1 AND revocada_en IS NULL`,
		tokenHashBytes(token),
	)
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// ValidateSession resolves a raw session token to the authenticated user. It
// enforces active user, unrevoked session and absolute expiration (no sliding
// renewal).
func (s *Service) ValidateSession(ctx context.Context, token string) (*UsuarioSesion, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNoAutenticado
	}
	var u UsuarioSesion
	var revocada *time.Time
	var expira time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.nombre_completo, u.login, u.rol, s.expira_en, s.revocada_en
		   FROM sesiones_usuario s
		   JOIN usuarios u ON u.id = s.usuario_id
		  WHERE s.token_hash = $1 AND u.activo = true`,
		tokenHashBytes(token),
	).Scan(&u.ID, &u.NombreCompleto, &u.Login, &u.Rol, &expira, &revocada)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoAutenticado
	}
	if err != nil {
		return nil, fmt.Errorf("validate session: %w", err)
	}
	if revocada != nil || !time.Now().Before(expira) {
		return nil, ErrNoAutenticado
	}
	return &u, nil
}

// RevokeUserSessions revokes every active session for a user. Used when
// deactivating or resetting a password.
func (s *Service) RevokeUserSessions(ctx context.Context, tx pgx.Tx, usuarioID int64) error {
	if tx == nil {
		_, err := s.pool.Exec(ctx,
			`UPDATE sesiones_usuario SET revocada_en = COALESCE(revocada_en, now())
			  WHERE usuario_id = $1 AND revocada_en IS NULL`,
			usuarioID,
		)
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE sesiones_usuario SET revocada_en = COALESCE(revocada_en, now())
		  WHERE usuario_id = $1 AND revocada_en IS NULL`,
		usuarioID,
	)
	return err
}

// BootstrapOwner creates the first owner only when the usuarios table is
// empty. Variables must all be present or the startup fails without printing
// the password. When users already exist the variables are ignored.
func (s *Service) BootstrapOwner(ctx context.Context, cfg BootstrapConfig) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bootstrap: %w", err)
	}
	defer rollback(ctx, tx)

	var count int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM usuarios`).Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit bootstrap skip: %w", err)
		}
		return nil
	}

	login := normalizeLogin(cfg.Login)
	nombre := strings.TrimSpace(cfg.Nombre)
	password := cfg.Password
	if login == "" || nombre == "" || strings.TrimSpace(password) == "" {
		return ErrBootstrapRequerido
	}
	if err := validatePassword(password); err != nil {
		// Surface a clear configuration error without logging the password.
		return fmt.Errorf("%w: %s", ErrBootstrapRequerido, err.Error())
	}

	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	var userID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO usuarios (nombre_completo, login, password_hash, rol, activo)
		 VALUES ($1, $2, $3, 'dueno', true)
		 RETURNING id`,
		nombre, login, hash,
	).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrLoginDuplicado
		}
		return fmt.Errorf("insert bootstrap owner: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'usuario_creado', 'usuario', $2, $3)`,
		userID, userID, `{"origen":"bootstrap","rol":"dueno"}`,
	); err != nil {
		return fmt.Errorf("audit bootstrap: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bootstrap: %w", err)
	}
	return nil
}
