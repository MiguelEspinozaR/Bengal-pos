package pos

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const cajaColumns = `id, estado, fecha_apertura, usuario_apertura_id, monto_inicial_bs,
	fecha_cierre, usuario_cierre_id, efectivo_esperado_bs, efectivo_contado_bs, diferencia_bs`

func scanCaja(row pgx.Row) (*CajaSesion, error) {
	var c CajaSesion
	err := row.Scan(&c.ID, &c.Estado, &c.FechaApertura, &c.UsuarioAperturaID, &c.MontoInicialBs,
		&c.FechaCierre, &c.UsuarioCierreID, &c.EfectivoEsperadoBs, &c.EfectivoContadoBs, &c.DiferenciaBs)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSinCaja
	}
	if err != nil {
		return nil, fmt.Errorf("scan cash session: %w", err)
	}
	return &c, nil
}

// OpenCaja opens the single allowed cash session and records its opening
// movement atomically (CJ-01/CJ-02). Only dueño/administrador may open it.
func (s *Service) OpenCaja(ctx context.Context, in OpenCajaInput) (*CajaSesion, error) {
	if in.MontoInicialBs < 0 {
		return nil, ErrMonto
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin open caja: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	caja, err := scanCaja(tx.QueryRow(ctx,
		`INSERT INTO caja_sesiones (estado, usuario_apertura_id, monto_inicial_bs)
		 VALUES ('abierta', $1, $2)
		 RETURNING `+cajaColumns,
		in.UsuarioID, in.MontoInicialBs,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrCajaAbierta
		}
		return nil, fmt.Errorf("open caja: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO caja_movimientos (sesion_id, tipo, monto_bs, usuario_id)
		 VALUES ($1, 'apertura', $2, $3)`,
		caja.ID, in.MontoInicialBs, in.UsuarioID,
	); err != nil {
		return nil, fmt.Errorf("insert opening cash movement: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'caja_apertura', 'caja_sesion', $2, jsonb_build_object('monto_inicial_bs', $3::bigint))`,
		in.UsuarioID, caja.ID, in.MontoInicialBs,
	); err != nil {
		return nil, fmt.Errorf("audit caja opening: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit open caja: %w", err)
	}
	return caja, nil
}

// GetOpenCaja returns the current open session, or nil when there is none.
func (s *Service) GetOpenCaja(ctx context.Context) (*CajaSesion, error) {
	caja, err := scanCaja(s.pool.QueryRow(ctx,
		`SELECT `+cajaColumns+` FROM caja_sesiones WHERE estado = 'abierta'`,
	))
	if errors.Is(err, ErrSinCaja) {
		return nil, nil
	}
	return caja, err
}

// CloseCaja computes expected cash from the session movements, always records
// counted cash and the difference, and seals the session (CJ-03).
func (s *Service) CloseCaja(ctx context.Context, in CloseCajaInput) (*CajaSesion, error) {
	if in.EfectivoContadoBs < 0 {
		return nil, ErrMonto
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin close caja: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := userRole(ctx, tx, in.UsuarioID); err != nil {
		return nil, err
	}

	var sesionID int64
	err = tx.QueryRow(ctx,
		`SELECT id FROM caja_sesiones WHERE estado = 'abierta' FOR UPDATE`,
	).Scan(&sesionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSinCaja
	}
	if err != nil {
		return nil, fmt.Errorf("load open caja for closing: %w", err)
	}

	var esperado int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(monto_bs), 0) FROM caja_movimientos WHERE sesion_id = $1`,
		sesionID,
	).Scan(&esperado); err != nil {
		return nil, fmt.Errorf("calculate expected cash: %w", err)
	}
	diferencia := in.EfectivoContadoBs - esperado

	caja, err := scanCaja(tx.QueryRow(ctx,
		`UPDATE caja_sesiones
		 SET estado = 'cerrada', fecha_cierre = now(), usuario_cierre_id = $2,
		     efectivo_esperado_bs = $3, efectivo_contado_bs = $4, diferencia_bs = $5
		 WHERE id = $1
		 RETURNING `+cajaColumns,
		sesionID, in.UsuarioID, esperado, in.EfectivoContadoBs, diferencia,
	))
	if err != nil {
		return nil, fmt.Errorf("close caja: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO caja_movimientos (sesion_id, tipo, monto_bs, usuario_id)
		 VALUES ($1, 'cierre', 0, $2)`,
		sesionID, in.UsuarioID,
	); err != nil {
		return nil, fmt.Errorf("insert closing cash movement: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'caja_cierre', 'caja_sesion', $2,
		         jsonb_build_object('efectivo_esperado_bs', $3::bigint, 'efectivo_contado_bs', $4::bigint, 'diferencia_bs', $5::bigint))`,
		in.UsuarioID, sesionID, esperado, in.EfectivoContadoBs, diferencia,
	); err != nil {
		return nil, fmt.Errorf("audit caja closing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit close caja: %w", err)
	}
	return caja, nil
}
