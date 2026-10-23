package testdb

import "testing"

// DATABASE_URL must be used verbatim: no provisioning and no rewriting.
func TestDSNReturnsDatabaseURLUnchanged(t *testing.T) {
	const want = "postgres://user:pass@db.example:5432/app?sslmode=require"
	t.Setenv("DATABASE_URL", want)
	t.Setenv("SKIP_INTEGRATION", "")

	if got := DSN(t); got != want {
		t.Fatalf("DSN() = %q, want DATABASE_URL %q", got, want)
	}
}

// With no local PostgreSQL binaries the integration tests must skip, not fail.
func TestDSNSkipsWithoutPostgresBinaries(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SKIP_INTEGRATION", "")

	reached := false
	t.Run("missing binaries", func(t *testing.T) {
		dsn(t, func() (string, string, bool) { return "", "", false })
		reached = true
	})
	if reached {
		t.Fatal("DSN did not skip when PostgreSQL binaries are missing")
	}
}
