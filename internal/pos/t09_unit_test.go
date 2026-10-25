package pos

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestT09_ValidatePasswordPolicy(t *testing.T) {
	valid := []string{"Clave123", "a1bcdefg", "Passw0rd!", strings.Repeat("a", 71) + "1"}
	for _, password := range valid {
		if err := validatePassword(password); err != nil {
			t.Fatalf("validatePassword(%q) = %v, want nil", password, err)
		}
	}
	invalid := []struct {
		password string
		want     error
	}{
		{"short1", ErrPasswordLongitud},
		{strings.Repeat("1", 73), ErrPasswordLongitud},
		{"abcdefgh", ErrPasswordDebil},
		{"12345678", ErrPasswordDebil},
		{"        ", ErrPasswordDebil},
	}
	for _, tc := range invalid {
		if err := validatePassword(tc.password); err != tc.want {
			t.Fatalf("validatePassword(%q) = %v, want %v", tc.password, err, tc.want)
		}
	}
}

func TestT09_NormalizeLogin(t *testing.T) {
	cases := map[string]string{
		"  Maria.Lopez ": "maria.lopez",
		"\tADMIN\n":      "admin",
		"":               "",
	}
	for raw, want := range cases {
		if got := normalizeLogin(raw); got != want {
			t.Fatalf("normalizeLogin(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestT09_PasswordHashIsBcryptCost12(t *testing.T) {
	const password = "Clave1234"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == password || strings.Contains(hash, password) {
		t.Fatalf("hash leaked the password: %q", hash)
	}
	prefix := hash[:4]
	if prefix != "$2a$" && prefix != "$2b$" && prefix != "$2y$" {
		t.Fatalf("hash prefix = %q, want a bcrypt prefix", prefix)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatal(err)
	}
	if cost != 12 {
		t.Fatalf("bcrypt cost = %d, want 12", cost)
	}
	if err := checkPassword(hash, password); err != nil {
		t.Fatalf("verify own hash: %v", err)
	}
	if err := checkPassword(hash, password+"x"); err == nil {
		t.Fatal("wrong password accepted")
	}
}

func TestT09_TokenIsRandomOpaqueAndHashed(t *testing.T) {
	token1, hash1, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	token2, hash2, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	if token1 == token2 {
		t.Fatal("tokens must be random and distinct")
	}
	if len(token1) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars (32 bytes)", len(token1))
	}
	if string(hash1) == token1 || string(hash1) == token2 {
		t.Fatal("stored hash must not be the raw token")
	}
	if string(hash1) == string(hash2) {
		t.Fatal("token hashes must differ")
	}
	if string(tokenHashBytes(token1)) != string(hash1) {
		t.Fatal("tokenHashBytes does not match generated hash")
	}
}
