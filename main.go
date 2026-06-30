package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dawit/golang-transaction-api/internal/handlers"
	"github.com/dawit/golang-transaction-api/internal/store"
)

func main() {
	dsn := env("DATABASE_URL", "postgres://txn:txn@localhost:5432/txn?sslmode=disable")
	addr := env("HTTP_ADDR", ":8080")

	db, err := store.OpenPostgres(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	st := store.NewPostgres(db)
	if err := st.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := st.SeedDemo(context.Background()); err != nil {
		log.Printf("seed demo: %v", err)
	}

	api := handlers.NewAPI(st)
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
