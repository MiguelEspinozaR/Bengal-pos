package pos

import "testing"

func TestNormalizeAreaNombre(t *testing.T) {
	got, err := normalizeAreaNombre("  Cocina  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Cocina" {
		t.Fatalf("nombre = %q", got)
	}

	if _, err := normalizeAreaNombre("   "); err != ErrAreaNombreRequerido {
		t.Fatalf("blank name error = %v", err)
	}
}

func TestValidateAreaOrden(t *testing.T) {
	if err := validateAreaOrden(0); err != nil {
		t.Fatalf("orden 0 rejected: %v", err)
	}
	if err := validateAreaOrden(10); err != nil {
		t.Fatalf("orden 10 rejected: %v", err)
	}
	if err := validateAreaOrden(-1); err != ErrAreaOrdenInvalido {
		t.Fatalf("negative orden error = %v", err)
	}
}

func TestNormalizePrintFormat(t *testing.T) {
	cases := map[string]string{
		"":      printFormatHTML,
		"html":  printFormatHTML,
		"HTML":  printFormatHTML,
		"texto": printFormatText,
		"text":  printFormatText,
	}
	for raw, want := range cases {
		got, err := normalizePrintFormat(raw)
		if err != nil {
			t.Fatalf("format %q: %v", raw, err)
		}
		if got != want {
			t.Fatalf("format %q = %q, want %q", raw, got, want)
		}
	}
	if _, err := normalizePrintFormat("pdf"); err != ErrFormatoImpresion {
		t.Fatalf("invalid format error = %v", err)
	}
}
