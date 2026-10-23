package pos

import "testing"

func TestBusinessMessagesMatchRules(t *testing.T) {
	cases := []struct {
		name string
		err  *AppError
		want string
	}{
		{"caja requerida", ErrCajaRequerida, "Debe abrir caja para cobrar en efectivo"},
		{"suma pagos", ErrSumaPagos, "La suma de los pagos confirmados no coincide con el total"},
		{"cambio", ErrCambio, "El efectivo recibido es menor que el monto en efectivo a pagar"},
		{"caja abierta", ErrCajaAbierta, "Ya hay una caja abierta"},
	}
	for _, tc := range cases {
		if tc.err.Message != tc.want {
			t.Fatalf("%s: message = %q, want %q", tc.name, tc.err.Message, tc.want)
		}
	}

	stock := stockError(-3)
	if stock.Message != "La salida dejará el stock en negativo (quedará en -3). ¿Desea continuar?" {
		t.Fatalf("stock message = %q", stock.Message)
	}
}
