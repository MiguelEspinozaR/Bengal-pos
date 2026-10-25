// Command api starts the local Point of Sale HTTP server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bengalpos/internal/database"
	"bengalpos/internal/httpapi"
	"bengalpos/internal/pos"
	"bengalpos/migrations"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	applied, err := database.ApplyMigrations(ctx, pool, migrations.FS)
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Printf("migrations applied: %d", applied)

	svc := pos.NewServiceWithConfig(pool, pos.Config{SessionTTL: httpapi.LoadSessionTTL()})
	if err := svc.BootstrapOwner(ctx, pos.BootstrapConfig{
		Login:    os.Getenv("ADMIN_INICIAL_LOGIN"),
		Password: os.Getenv("ADMIN_INICIAL_PASSWORD"),
		Nombre:   os.Getenv("ADMIN_INICIAL_NOMBRE"),
	}); err != nil {
		// The error never contains the password or its hash.
		log.Fatalf("bootstrap del primer dueño: %v", err)
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewServer(svc),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http server: %v", err)
	}
}
