package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func normalizeNombre(raw string) string {
	return strings.TrimSpace(raw)
}

func allowedRolesFor(actorRol string) map[string]bool {
	switch actorRol {
	case "dueno":
		return map[string]bool{"administrador": true, "encargado": true, "empleado": true}
	case "administrador":
		return map[string]bool{"encargado": true, "empleado": true}
	default:
		return map[string]bool{}
	}
}

func canManage(actorRol, targetRol string) bool {
	if actorRol == "dueno" {
		return true
	}
	if actorRol == "administrador" {
		return targetRol == "encargado" || targetRol == "empleado"
	}
	return false
}

func scanUsuario(row pgx.Row) (Usuario, error) {
	var u Usuario
	err := row.Scan(&u.ID, &u.NombreCompleto, &u.Login, &u.Rol, &u.Activo, &u.CreadoEn, &u.ActualizadoEn)
	return u, err
}

func (s *Service) getUser(ctx context.Context, tx pgx.Tx, id int64) (Usuario, error) {
	row := tx.QueryRow(ctx,
		`SELECT id, nombre_completo, login, rol, activo, creado_en, actualizado_en
		   FROM usuarios WHERE id = $1 FOR UPDATE`,
		id,
	)
	u, err := scanUsuario(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Usuario{}, ErrUsuarioNoExiste
	}
	return u, err
}

// ListUsuarios lists users without password_hash. Only active are returned
// unless incluirInactivos is true. Caller must enforce authorization.
func (s *Service) ListUsuarios(ctx context.Context, incluirInactivos bool) ([]Usuario, error) {
	query := `SELECT id, nombre_completo, login, rol, activo, creado_en, actualizado_en
		        FROM usuarios`
	if !incluirInactivos {
		query += ` WHERE activo = true`
	}
	query += ` ORDER BY id`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	out := make([]Usuario, 0)
	for rows.Next() {
		u, err := scanUsuario(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return out, nil
}

// CreateUsuario creates a user with an immutable role. Dueño creates
// administradores/encargados/empleados; administrador creates only
// encargados/empleados; empleados/encargados cannot create users.
func (s *Service) CreateUsuario(ctx context.Context, in CreateUsuarioInput) (*Usuario, error) {
	nombre := normalizeNombre(in.NombreCompleto)
	login := normalizeLogin(in.Login)
	rol := strings.TrimSpace(in.Rol)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create user: %w", err)
	}
	defer rollback(ctx, tx)

	actor, err := s.getUser(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !actor.Activo {
		return nil, ErrNoAutorizado
	}
	if !canAuthorize(actor.Rol) {
		return nil, ErrNoAutorizado
	}
	allowed := allowedRolesFor(actor.Rol)
	if !allowed[rol] {
		if rol == "dueno" || (actor.Rol == "administrador" && rol == "administrador") {
			return nil, ErrRolNoPermitido
		}
		if rol != "administrador" && rol != "encargado" && rol != "empleado" {
			return nil, ErrRolInvalido
		}
		return nil, ErrRolNoPermitido
	}
	if nombre == "" {
		return nil, ErrNombreUsuarioRequerido
	}
	if login == "" {
		return nil, ErrLoginRequerido
	}
	if err := validatePassword(in.Password); err != nil {
		return nil, err
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	var u Usuario
	err = tx.QueryRow(ctx,
		`INSERT INTO usuarios (nombre_completo, login, password_hash, rol, activo)
		 VALUES ($1, $2, $3, $4, true)
		 RETURNING id, nombre_completo, login, rol, activo, creado_en, actualizado_en`,
		nombre, login, hash, rol,
	).Scan(&u.ID, &u.NombreCompleto, &u.Login, &u.Rol, &u.Activo, &u.CreadoEn, &u.ActualizadoEn)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrLoginDuplicado
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	auditData, err := json.Marshal(map[string]string{"rol": u.Rol, "login": u.Login})
	if err != nil {
		return nil, fmt.Errorf("encode create user audit: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'usuario_creado', 'usuario', $2, $3::jsonb)`,
		actor.ID, u.ID, string(auditData),
	); err != nil {
		return nil, fmt.Errorf("audit create user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create user: %w", err)
	}
	return &u, nil
}

// UpdateUsuario changes name, activo or resets password; role cannot change.
// Only dueño can modify administrators; administrators can modify
// encargados/empleados only; nobody may deactivate themselves or leave no
// active dueño.
func (s *Service) UpdateUsuario(ctx context.Context, userID int64, in UpdateUsuarioInput) (*Usuario, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update user: %w", err)
	}
	defer rollback(ctx, tx)

	actor, err := s.getUser(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !actor.Activo || !canAuthorize(actor.Rol) {
		return nil, ErrNoAutorizado
	}
	target, err := s.getUser(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if actor.ID != target.ID && !canManage(actor.Rol, target.Rol) {
		return nil, ErrNoAutorizado
	}
	if in.Rol != nil {
		return nil, ErrRolInmutable
	}
	if in.Activo != nil && *in.Activo == false && actor.ID == target.ID {
		return nil, ErrAutoDesactivacion
	}
	if in.Activo != nil && *in.Activo == false {
		if target.Rol == "dueno" {
			var activeDueno int64
			if err := tx.QueryRow(ctx,
				`SELECT COUNT(*) FROM usuarios WHERE rol = 'dueno' AND activo = true AND id <> $1`,
				target.ID,
			).Scan(&activeDueno); err != nil {
				return nil, fmt.Errorf("count active duenos: %w", err)
			}
			if activeDueno < 1 {
				return nil, ErrUltimoDueno
			}
		}
	}

	set := make([]string, 0, 4)
	args := make([]any, 0, 5)
	idx := 1
	if in.NombreCompleto != nil {
		nombre := normalizeNombre(*in.NombreCompleto)
		if nombre == "" {
			return nil, ErrNombreUsuarioRequerido
		}
		set = append(set, fmt.Sprintf("nombre_completo = $%d", idx))
		args = append(args, nombre)
		idx++
	}
	if in.Activo != nil {
		set = append(set, fmt.Sprintf("activo = $%d", idx))
		args = append(args, *in.Activo)
		idx++
	}
	var passwordReset bool
	if in.Password != nil {
		if err := validatePassword(*in.Password); err != nil {
			return nil, err
		}
		hash, err := hashPassword(*in.Password)
		if err != nil {
			return nil, err
		}
		set = append(set, fmt.Sprintf("password_hash = $%d", idx))
		args = append(args, hash)
		idx++
		passwordReset = true
	}
	if len(set) == 0 {
		// Nothing to change; return current user.
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit update user: %w", err)
		}
		return &target, nil
	}
	set = append(set, "actualizado_en = now()")
	args = append(args, userID)
	query := fmt.Sprintf(`UPDATE usuarios SET %s WHERE id = $%d
		RETURNING id, nombre_completo, login, rol, activo, creado_en, actualizado_en`, strings.Join(set, ", "), idx)

	var u Usuario
	if err := tx.QueryRow(ctx, query, args...).Scan(&u.ID, &u.NombreCompleto, &u.Login, &u.Rol, &u.Activo, &u.CreadoEn, &u.ActualizadoEn); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if in.Activo != nil && *in.Activo == false {
		if err := s.RevokeUserSessions(ctx, tx, userID); err != nil {
			return nil, fmt.Errorf("revoke sessions on deactivate: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
			 VALUES ($1, 'usuario_desactivado', 'usuario', $2, $3)`,
			actor.ID, userID, `{"activo":false}`,
		); err != nil {
			return nil, fmt.Errorf("audit deactivate: %w", err)
		}
	} else if in.Activo != nil && *in.Activo == true && target.Activo == false {
		if _, err := tx.Exec(ctx,
			`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
			 VALUES ($1, 'usuario_activado', 'usuario', $2, $3)`,
			actor.ID, userID, `{"activo":true}`,
		); err != nil {
			return nil, fmt.Errorf("audit activate: %w", err)
		}
	}
	if passwordReset {
		if err := s.RevokeUserSessions(ctx, tx, userID); err != nil {
			return nil, fmt.Errorf("revoke sessions on password reset: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
			 VALUES ($1, 'usuario_reset_password', 'usuario', $2, $3)`,
			actor.ID, userID, `{}`,
		); err != nil {
			return nil, fmt.Errorf("audit reset password: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update user: %w", err)
	}
	return &u, nil
}
