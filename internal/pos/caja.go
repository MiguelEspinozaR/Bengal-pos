package pos

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const cajaColumns = `id, estado, fecha_apertura, usuario_apertura_id, monto_inicial_bs,
	fecha_cierre, usuario_cierre_id, efectivo_esperado_bs, efectivo_contado_bs, diferencia_bs`

const avisoVentasEnProceso = "Hay ventas en proceso sin confirmar"

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

// normalizeCajaMovimiento validates and signs an entrada/salida. The server,
// never the client, applies the sign: entradas add cash and salidas remove it.
func normalizeCajaMovimiento(tipo string, montoBs int64, motivo string) (string, int64, string, error) {
	tipo = strings.ToLower(strings.TrimSpace(tipo))
	if tipo != "entrada" && tipo != "salida" {
		return "", 0, "", ErrTipoMovimiento
	}
	if montoBs <= 0 {
		return "", 0, "", ErrMonto
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return "", 0, "", ErrMotivoRequerido
	}
	if tipo == "salida" {
		montoBs = -montoBs
	}
	return tipo, montoBs, motivo, nil
}

// AddCajaMovimiento registers an entrada or salida with its motive and user in
// the open session (RF-21). Only dueño/administrador may do it (RT-05).
func (s *Service) AddCajaMovimiento(ctx context.Context, in CajaMovimientoInput) (*CajaMovimiento, error) {
	tipo, montoBs, motivo, err := normalizeCajaMovimiento(in.Tipo, in.MontoBs, in.Motivo)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin cash movement: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
	}

	var sesionID int64
	err = tx.QueryRow(ctx,
		`SELECT id FROM caja_sesiones WHERE estado = 'abierta' FOR UPDATE`,
	).Scan(&sesionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSinCaja
	}
	if err != nil {
		return nil, fmt.Errorf("load open caja for movement: %w", err)
	}

	var movimiento CajaMovimiento
	err = tx.QueryRow(ctx,
		`INSERT INTO caja_movimientos (sesion_id, tipo, monto_bs, motivo, usuario_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, sesion_id, tipo, monto_bs, motivo, usuario_id, venta_id, creado_en`,
		sesionID, tipo, montoBs, motivo, in.UsuarioID,
	).Scan(&movimiento.ID, &movimiento.SesionID, &movimiento.Tipo, &movimiento.MontoBs,
		&movimiento.Motivo, &movimiento.UsuarioID, &movimiento.VentaID, &movimiento.CreadoEn)
	if err != nil {
		return nil, fmt.Errorf("insert cash movement: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'caja_movimiento', 'caja_movimiento', $2,
		         jsonb_build_object('sesion_id', $3::bigint, 'tipo', $4::text,
		                            'monto_bs', $5::bigint, 'motivo', $6::text))`,
		in.UsuarioID, movimiento.ID, sesionID, tipo, montoBs, motivo,
	); err != nil {
		return nil, fmt.Errorf("audit cash movement: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cash movement: %w", err)
	}
	return &movimiento, nil
}

// ListCajaSesiones returns the permanent session history, newest first (RF-23).
func (s *Service) ListCajaSesiones(ctx context.Context) ([]CajaSesion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+cajaColumns+` FROM caja_sesiones
		 ORDER BY fecha_apertura DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list cash sessions: %w", err)
	}
	defer rows.Close()

	sesiones := []CajaSesion{}
	for rows.Next() {
		var c CajaSesion
		if err := rows.Scan(&c.ID, &c.Estado, &c.FechaApertura, &c.UsuarioAperturaID, &c.MontoInicialBs,
			&c.FechaCierre, &c.UsuarioCierreID, &c.EfectivoEsperadoBs, &c.EfectivoContadoBs, &c.DiferenciaBs); err != nil {
			return nil, fmt.Errorf("scan cash session history: %w", err)
		}
		sesiones = append(sesiones, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cash session history: %w", err)
	}
	return sesiones, nil
}

// ListCajaMovimientos returns every movement of a session in chronological
// order, including apertura, ventas en efectivo, entradas, salidas and cierre.
func (s *Service) ListCajaMovimientos(ctx context.Context, sesionID int64) ([]CajaMovimiento, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM caja_sesiones WHERE id = $1)`, sesionID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check cash session: %w", err)
	}
	if !exists {
		return nil, ErrSesionNoExiste
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, sesion_id, tipo, monto_bs, motivo, usuario_id, venta_id, creado_en
		 FROM caja_movimientos WHERE sesion_id = $1
		 ORDER BY creado_en, id`, sesionID)
	if err != nil {
		return nil, fmt.Errorf("list cash movements: %w", err)
	}
	defer rows.Close()

	movimientos := []CajaMovimiento{}
	for rows.Next() {
		var m CajaMovimiento
		if err := rows.Scan(&m.ID, &m.SesionID, &m.Tipo, &m.MontoBs, &m.Motivo,
			&m.UsuarioID, &m.VentaID, &m.CreadoEn); err != nil {
			return nil, fmt.Errorf("scan cash movement: %w", err)
		}
		movimientos = append(movimientos, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cash movements: %w", err)
	}
	return movimientos, nil
}

// CloseCaja computes expected cash from the session movements, always records
// counted cash and the difference, seals the session and returns the CJ-04
// warning without blocking the close (CJ-03/CJ-04).
func (s *Service) CloseCaja(ctx context.Context, in CloseCajaInput) (*CajaCierreResult, error) {
	if in.EfectivoContadoBs < 0 {
		return nil, ErrMonto
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin close caja: %w", err)
	}
	defer rollback(ctx, tx)

	rol, err := userRole(ctx, tx, in.UsuarioID)
	if err != nil {
		return nil, err
	}
	if !canAuthorize(rol) {
		return nil, ErrNoAutorizado
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

	// CJ-04: drafts never contribute to expected cash and never block closing.
	ventas := []CajaVentaEnProceso{}
	rows, err := tx.Query(ctx,
		`SELECT id, usuario_id, total_bs, fecha FROM ventas
		 WHERE estado = 'en_proceso'
		 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load sales in progress: %w", err)
	}
	for rows.Next() {
		var venta CajaVentaEnProceso
		if err := rows.Scan(&venta.ID, &venta.UsuarioID, &venta.TotalBs, &venta.Fecha); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan sale in progress: %w", err)
		}
		ventas = append(ventas, venta)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate sales in progress: %w", err)
	}
	rows.Close()

	var esperado int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(monto_bs), 0) FROM caja_movimientos WHERE sesion_id = $1`,
		sesionID,
	).Scan(&esperado); err != nil {
		return nil, fmt.Errorf("calculate expected cash: %w", err)
	}
	diferencia := in.EfectivoContadoBs - esperado

	// Insert the 0-amount closing movement while the session is still open, so
	// the database trigger can reject every insert after sealing.
	if _, err := tx.Exec(ctx,
		`INSERT INTO caja_movimientos (sesion_id, tipo, monto_bs, usuario_id)
		 VALUES ($1, 'cierre', 0, $2)`,
		sesionID, in.UsuarioID,
	); err != nil {
		return nil, fmt.Errorf("insert closing cash movement: %w", err)
	}

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
		`INSERT INTO auditoria (usuario_id, accion, entidad_tipo, entidad_id, datos)
		 VALUES ($1, 'caja_cierre', 'caja_sesion', $2,
		         jsonb_build_object('efectivo_esperado_bs', $3::bigint,
		                            'efectivo_contado_bs', $4::bigint,
		                            'diferencia_bs', $5::bigint,
		                            'ventas_en_proceso', $6::bigint))`,
		in.UsuarioID, sesionID, esperado, in.EfectivoContadoBs, diferencia, int64(len(ventas)),
	); err != nil {
		return nil, fmt.Errorf("audit caja closing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit close caja: %w", err)
	}

	result := &CajaCierreResult{
		CajaSesion:             *caja,
		VentasEnProceso:        int64(len(ventas)),
		VentasEnProcesoDetalle: ventas,
	}
	if len(ventas) > 0 {
		result.Aviso = avisoVentasEnProceso
	}
	return result, nil
}
