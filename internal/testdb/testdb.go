// Package testdb provides a reproducible PostgreSQL connection for integration
// tests.
//
// When DATABASE_URL is set, DSN returns it unchanged and provisions nothing.
// Otherwise DSN starts a throwaway PostgreSQL cluster with the local binaries
// found in PATH or /usr/lib/postgresql/*/bin (newest version preferred), creates
// a dedicated database, applies the project migrations and returns a DSN that
// points at it. The cluster is stopped and its temporary directory removed by
// t.Cleanup, including when provisioning or the test fails.
//
// Integration tests can be forced to skip with SKIP_INTEGRATION=1. If no local
// PostgreSQL binaries are available the test is skipped with an explanatory
// message instead of failing the unit suite.
package testdb

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"bengalpos/internal/database"
	"bengalpos/migrations"
)

const (
	startTimeout = 30 * time.Second
	databaseName = "bengalpos_test"
)

// DSN returns a PostgreSQL DSN for an integration test.
//
// DATABASE_URL wins: it is returned as-is and no local cluster is started.
// Otherwise an ephemeral cluster is provisioned and registered for cleanup.
// The test is skipped when SKIP_INTEGRATION=1 or when PostgreSQL binaries are
// not installed locally.
func DSN(t *testing.T) string {
	t.Helper()
	return dsn(t, findPostgresBinaries)
}

// dsn carries the provisioning logic and accepts the binary lookup so tests can
// exercise the missing-binaries branch without uninstalling PostgreSQL.
func dsn(t *testing.T, findBinaries func() (dir, version string, ok bool)) string {
	t.Helper()

	if os.Getenv("SKIP_INTEGRATION") == "1" {
		t.Skip("integration tests disabled by SKIP_INTEGRATION=1")
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}

	binDir, version, ok := findBinaries()
	if !ok {
		t.Skip("PostgreSQL binaries not found in PATH or /usr/lib/postgresql/*/bin; install PostgreSQL or set DATABASE_URL to run integration tests (SKIP_INTEGRATION=1 forces skip)")
	}

	port, err := freePort()
	if err != nil {
		t.Fatalf("testdb: find free port: %v", err)
	}

	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	logPath := filepath.Join(tempDir, "postgres.log")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("testdb: create data directory: %v", err)
	}

	started := false
	t.Cleanup(func() {
		if started {
			out, err := runCommand(binDir, 15*time.Second, "pg_ctl",
				"-D", dataDir, "-m", "immediate", "-w", "-t", "10", "stop")
			if err != nil {
				t.Logf("testdb: stop ephemeral PostgreSQL: %v\n%s", err, out)
			}
		}
		// t.TempDir also removes this directory after our cleanup runs; removing
		// it explicitly documents and enforces the guarantee on failures.
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("testdb: remove temporary cluster directory: %v", err)
		}
	})

	// Trust authentication is acceptable because the server only listens on
	// 127.0.0.1 and the cluster is never reachable outside this machine.
	initOut, err := runCommand(binDir, startTimeout, "initdb",
		"-D", dataDir,
		"-U", "postgres",
		"-A", "trust",
		"--no-locale",
		"--encoding=UTF8",
	)
	if err != nil {
		t.Fatalf("testdb: initdb (PostgreSQL %s): %v\n%s", version, err, initOut)
	}

	serverOptions := fmt.Sprintf("-p %d -c listen_addresses=127.0.0.1 -c unix_socket_directories=%q",
		port, tempDir)
	startOut, err := runCommand(binDir, startTimeout, "pg_ctl",
		"-D", dataDir,
		"-l", logPath,
		"-o", serverOptions,
		"-w",
		"-t", strconv.Itoa(int(startTimeout.Seconds())),
		"start",
	)
	if err != nil {
		logData, _ := os.ReadFile(logPath)
		t.Fatalf("testdb: start ephemeral PostgreSQL: %v\n%s\nserver log:\n%s", err, startOut, logData)
	}
	started = true

	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	adminDSN := fmt.Sprintf("postgres://postgres@127.0.0.1:%d/postgres?sslmode=disable", port)
	adminPool, err := database.NewPool(ctx, adminDSN)
	if err != nil {
		t.Fatalf("testdb: connect to ephemeral PostgreSQL: %v", err)
	}
	_, createErr := adminPool.Exec(ctx, "CREATE DATABASE "+databaseName)
	adminPool.Close()
	if createErr != nil {
		t.Fatalf("testdb: create test database: %v", createErr)
	}

	dsn := fmt.Sprintf("postgres://postgres@127.0.0.1:%d/%s?sslmode=disable", port, databaseName)
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("testdb: connect to test database: %v", err)
	}
	defer pool.Close()

	if _, err := database.ApplyMigrations(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("testdb: apply migrations: %v", err)
	}

	return dsn
}

// findPostgresBinaries returns the newest directory that contains the initdb,
// pg_ctl and postgres executables, together with its version label.
func findPostgresBinaries() (dir, version string, ok bool) {
	type candidate struct {
		dir     string
		major   int
		version string
	}

	seen := make(map[string]bool)
	var candidates []candidate

	add := func(candidateDir string) {
		if candidateDir == "" || seen[candidateDir] {
			return
		}
		seen[candidateDir] = true
		if !hasPostgresBinaries(candidateDir) {
			return
		}
		major, label := postgresVersion(candidateDir)
		candidates = append(candidates, candidate{dir: candidateDir, major: major, version: label})
	}

	for _, candidateDir := range filepath.SplitList(os.Getenv("PATH")) {
		add(candidateDir)
	}
	// Debian/Ubuntu install versioned servers here even when the versioned bin
	// directory is not on PATH.
	versionedDirs, _ := filepath.Glob("/usr/lib/postgresql/*/bin")
	for _, candidateDir := range versionedDirs {
		add(candidateDir)
	}

	if len(candidates) == 0 {
		return "", "", false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].major != candidates[j].major {
			return candidates[i].major > candidates[j].major
		}
		return candidates[i].version > candidates[j].version
	})
	return candidates[0].dir, candidates[0].version, true
}

func hasPostgresBinaries(dir string) bool {
	for _, name := range []string{"initdb", "pg_ctl", "postgres"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			return false
		}
	}
	return true
}

func postgresVersion(dir string) (int, string) {
	// /usr/lib/postgresql/<major>/bin encodes the major version in the path.
	slashPath := filepath.ToSlash(dir)
	if idx := strings.Index(slashPath, "/postgresql/"); idx >= 0 {
		rest := slashPath[idx+len("/postgresql/"):]
		segment := strings.SplitN(rest, "/", 2)[0]
		if major, err := strconv.Atoi(segment); err == nil {
			return major, segment
		}
	}

	out, err := exec.Command(filepath.Join(dir, "initdb"), "--version").Output()
	if err != nil {
		return 0, ""
	}
	fields := strings.Fields(string(out))
	for _, field := range fields {
		trimmed := strings.TrimSuffix(field, ",")
		if major, err := strconv.Atoi(strings.SplitN(trimmed, ".", 2)[0]); err == nil {
			return major, trimmed
		}
	}
	return 0, strings.TrimSpace(string(out))
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// runCommand executes a PostgreSQL binary from binDir so pg_ctl can locate the
// matching postgres executable, and returns combined stdout/stderr on failure.
func runCommand(binDir string, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	path := binDir
	if existing := os.Getenv("PATH"); existing != "" {
		path += string(os.PathListSeparator) + existing
	}
	// Replace PATH instead of appending a duplicate variable: pg_ctl locates the
	// matching postgres binary through it.
	env := make([]string, 0, len(os.Environ())+1)
	for _, variable := range os.Environ() {
		if strings.HasPrefix(variable, "PATH=") {
			continue
		}
		env = append(env, variable)
	}
	env = append(env, "PATH="+path)

	cmd := exec.CommandContext(ctx, filepath.Join(binDir, name), args...)
	cmd.Env = env
	return cmd.CombinedOutput()
}
