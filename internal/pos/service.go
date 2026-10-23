// Package pos implements the sales, split-payment, inventory and cash
// application use cases on top of PostgreSQL. Amounts are always integer minor
// units of Bs.
package pos

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bengalpos/internal/domain"
)

// Service is the application boundary used by the HTTP transport.
type Service struct {
	pool *pgxpool.Pool
}

// NewService builds a sales/cash service around a pgx pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

type dbQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func mapDomainError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrCantidadInvalida):
		return ErrCantidad
	case errors.Is(err, domain.ErrEfectivoInsuficiente):
		return ErrCambio
	case errors.Is(err, domain.ErrSumaPagos):
		return ErrSumaPagos
	case errors.Is(err, domain.ErrMontoInvalido):
		return ErrMonto
	case errors.Is(err, domain.ErrAreaInvalida):
		return ErrAreaInvalida
	default:
		return err
	}
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func userRole(ctx context.Context, q dbQuerier, usuarioID int64) (string, error) {
	var rol string
	err := q.QueryRow(ctx, `SELECT rol FROM usuarios WHERE id = $1 AND activo`, usuarioID).Scan(&rol)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoAutorizado
	}
	if err != nil {
		return "", fmt.Errorf("load user role: %w", err)
	}
	return rol, nil
}

func canAuthorize(rol string) bool {
	return rol == "dueno" || rol == "administrador"
}
