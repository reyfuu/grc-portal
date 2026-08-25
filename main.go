package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"grcportal/internal/auth"
	"grcportal/internal/db"
	"grcportal/internal/handlers"
	"grcportal/internal/web"
)

func main() {
	dsn := env("DATABASE_URL", "postgres://localhost:5432/grc?sslmode=disable")
	addr := env("ADDR", ":8081") // 8080 often taken by local Apache; override with ADDR
	// Secure cookies by default; set DEV=1 for local http so the cookie is still sent.
	secure := os.Getenv("DEV") == ""

	database, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := seedAdmin(database); err != nil {
		log.Fatalf("seed admin: %v", err)
	}

	h := handlers.New(database, secure)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.HandleFunc("GET /", web.SPAHandler())

	srv := &http.Server{
		Addr:         addr,
		Handler:      auth.Middleware(database)(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// seedAdmin creates the first admin from ADMIN_EMAIL/ADMIN_PASSWORD when there are
// no portal users yet. No-op once any user exists (no self-registration).
func seedAdmin(database *sql.DB) error {
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM portal_users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	pw := os.Getenv("ADMIN_PASSWORD")
	if email == "" || pw == "" {
		log.Println("no portal users and ADMIN_EMAIL/ADMIN_PASSWORD not set; skipping admin seed")
		return nil
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if _, err := database.Exec(
		`INSERT INTO portal_users(email, password_hash, display_name, role) VALUES($1,$2,'Administrator','admin')`,
		email, hash); err != nil {
		return err
	}
	log.Printf("seeded admin user %s", email)
	return nil
}
