package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"wireguard-ui/internal/api"
	"wireguard-ui/internal/auth"
	"wireguard-ui/internal/config"
	"wireguard-ui/internal/web"
	"wireguard-ui/internal/wg"
)

func main() {
	cfg := config.Load()

	// Data dir holds the panel's own state (credentials, client private keys).
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		log.Fatalf("create data dir %s: %v", cfg.DataDir, err)
	}
	mgr, err := auth.NewManager(filepath.Join(cfg.DataDir, "credentials.json"), cfg.AdminUser, cfg.AdminPass)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}
	secrets, err := wg.OpenSecrets(filepath.Join(cfg.DataDir, "secrets.json"))
	if err != nil {
		log.Fatalf("open secrets store: %v", err)
	}
	settings, err := wg.OpenSettings(filepath.Join(cfg.DataDir, "settings.json"), cfg.EndpointHost)
	if err != nil {
		log.Fatalf("open settings store: %v", err)
	}

	svc := wg.NewService(cfg.WgConfPath, cfg.IfaceName(), secrets, settings)
	handlers := api.NewHandlers(cfg, mgr, svc)

	mux := http.NewServeMux()
	handlers.Mount(mux, mgr)
	// Everything not under /api is served by the embedded SPA.
	mux.Handle("/", web.Handler())

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		log.Printf("wireguard-ui listening on %s (admin user: %s)", cfg.Addr, cfg.AdminUser)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

// logRequests is a tiny access-log middleware.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
