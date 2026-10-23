// Package domain contains pure business rules for sales and payments. It never
// uses floating point arithmetic: every amount is an integer in the minimum
// unit of Bs (RT-01).
package domain

import (
	"errors"
	"math"
)

var (
	// ErrCantidadInvalida is returned for zero, negative or overflowing item
	// quantities (RT-02).
	ErrCantidadInvalida = errors.New("La cantidad debe ser un entero mayor que cero")
	// ErrSumaPagos has the exact PG-02 message.
	ErrSumaPagos = errors.New("La suma de los pagos confirmados no coincide con el total")
	// ErrEfectivoInsuficiente has the exact PG-01 message.
	ErrEfectivoInsuficiente = errors.New("El efectivo recibido es menor que el monto en efectivo a pagar")
	// ErrMontoInvalido is returned when a monetary value cannot satisfy its
	// non-negative/positive invariant.
	ErrMontoInvalido = errors.New("El monto debe ser un entero válido")
)

// SaleLine is the minimum information required to price one sale line.
type SaleLine struct {
	Cantidad         int64
	PrecioUnitarioBs int64
	DescuentoBs      int64
}

// Subtotal returns cantidad × precio − descuento using integer arithmetic.
func (l SaleLine) Subtotal() (int64, error) {
	if l.Cantidad <= 0 {
		return 0, ErrCantidadInvalida
	}
	if l.PrecioUnitarioBs < 0 || l.DescuentoBs < 0 {
		return 0, ErrMontoInvalido
	}
	if l.PrecioUnitarioBs != 0 && l.Cantidad > math.MaxInt64/l.PrecioUnitarioBs {
		return 0, ErrMontoInvalido
	}
	base := l.Cantidad * l.PrecioUnitarioBs
	if l.DescuentoBs > base {
		return 0, ErrMontoInvalido
	}
	return base - l.DescuentoBs, nil
}

// SaleTotal sums every line subtotal, guarding against overflow and negative
// results.
func SaleTotal(lines []SaleLine) (int64, error) {
	if len(lines) == 0 {
		return 0, errors.New("La venta debe tener al menos un ítem")
	}
	var total int64
	for _, line := range lines {
		subtotal, err := line.Subtotal()
		if err != nil {
			return 0, err
		}
		if subtotal > math.MaxInt64-total {
			return 0, ErrMontoInvalido
		}
		total += subtotal
	}
	return total, nil
}

// CashChange validates a cash payment and returns the change in integer minor
// units (PG-01).
func CashChange(montoEfectivoBs, recibidoBs int64) (int64, error) {
	if montoEfectivoBs <= 0 || recibidoBs < 0 {
		return 0, ErrMontoInvalido
	}
	if recibidoBs < montoEfectivoBs {
		return 0, ErrEfectivoInsuficiente
	}
	return recibidoBs - montoEfectivoBs, nil
}

// ValidateConfirmedPayments enforces Σ(confirmed parts) = total (PG-02).
func ValidateConfirmedPayments(totalBs int64, confirmedParts []int64) error {
	if totalBs < 0 {
		return ErrMontoInvalido
	}
	var sum int64
	for _, part := range confirmedParts {
		if part <= 0 {
			return ErrMontoInvalido
		}
		if part > math.MaxInt64-sum {
			return ErrMontoInvalido
		}
		sum += part
	}
	if sum != totalBs {
		return ErrSumaPagos
	}
	return nil
}

// ValidateDiscount enforces 0 ≤ descuento ≤ subtotal (RT-05).
func ValidateDiscount(descuentoBs, subtotalBs int64) error {
	if descuentoBs < 0 || subtotalBs < 0 || descuentoBs > subtotalBs {
		return ErrMontoInvalido
	}
	return nil
}
