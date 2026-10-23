package domain

import (
	"errors"
	"testing"
)

func TestSaleLineSubtotalAndTotal(t *testing.T) {
	lines := []SaleLine{
		{Cantidad: 2, PrecioUnitarioBs: 2500},
		{Cantidad: 1, PrecioUnitarioBs: 5000, DescuentoBs: 500},
	}
	total, err := SaleTotal(lines)
	if err != nil {
		t.Fatalf("SaleTotal() error = %v", err)
	}
	if want := int64(9500); total != want {
		t.Fatalf("SaleTotal() = %d, want %d", total, want)
	}
}

func TestSaleTotalRejectsInvalidQuantities(t *testing.T) {
	for _, lines := range [][]SaleLine{
		nil,
		{{Cantidad: 0, PrecioUnitarioBs: 100}},
		{{Cantidad: -1, PrecioUnitarioBs: 100}},
		{{Cantidad: 1, PrecioUnitarioBs: -100}},
		{{Cantidad: 1, PrecioUnitarioBs: 100, DescuentoBs: 101}},
	} {
		if _, err := SaleTotal(lines); err == nil {
			t.Fatalf("SaleTotal(%+v) unexpectedly succeeded", lines)
		}
	}
}

func TestCashChange(t *testing.T) {
	change, err := CashChange(4500, 5000)
	if err != nil {
		t.Fatalf("CashChange() error = %v", err)
	}
	if change != 500 {
		t.Fatalf("CashChange() = %d, want 500", change)
	}

	if _, err := CashChange(4500, 4500); err != nil {
		t.Fatalf("exact cash should produce zero change without error: %v", err)
	}

	if _, err := CashChange(4500, 4499); !errors.Is(err, ErrEfectivoInsuficiente) {
		t.Fatalf("CashChange() error = %v, want ErrEfectivoInsuficiente", err)
	}
}

func TestValidateConfirmedPayments(t *testing.T) {
	if err := ValidateConfirmedPayments(10000, []int64{6000, 4000}); err != nil {
		t.Fatalf("expected split payment to balance: %v", err)
	}
	for _, parts := range [][]int64{{6000}, {6000, 4000, 1}, {}, {0, 10000}} {
		if err := ValidateConfirmedPayments(10000, parts); err == nil {
			t.Fatalf("parts %v unexpectedly balanced", parts)
		}
	}
}

func TestValidateDiscount(t *testing.T) {
	if err := ValidateDiscount(500, 5000); err != nil {
		t.Fatalf("valid discount rejected: %v", err)
	}
	if err := ValidateDiscount(5001, 5000); err == nil {
		t.Fatal("discount above subtotal must be rejected")
	}
}
