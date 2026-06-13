package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"wirenest/internal/api"
	"wirenest/internal/auth"
	"wirenest/internal/config"
	"wirenest/internal/update"
	"wirenest/internal/web"
	"wirenest/internal/wg"
)

// version is injected at build time via -ldflags "-X main.version=v0.1.2".
// The self-updater also runs the downloaded binary with `-version` to validate it.
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-version" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(version)
		return
	}

	cfg := config.Load()
	cfg.Version = version

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
	limits, err := wg.OpenLimits(filepath.Join(cfg.DataDir, "limits.json"))
	if err != nil {
		log.Fatalf("open limits store: %v", err)
	}

	svc := wg.NewService(cfg.WgConfPath, cfg.IfaceName(), secrets, settings, limits)
	handlers := api.NewHandlers(cfg, mgr, svc)

	mux := http.NewServeMux()
	handlers.Mount(mux, mgr)
	// Everything not under /api is served by the embedded SPA.
	mux.Handle("/", web.Handler())

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           logRequests(gzipMiddleware(mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		log.Printf("wirenest listening on %s (admin user: %s)", cfg.Addr, cfg.AdminUser)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Keep the companion `wirenest` CLI in sync on startup (best-effort), so the
	// management menu always reflects the running version even if the panel was
	// updated by an older binary that didn't refresh it.
	go func() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		update.RefreshCLI(ctx, cfg.Repo, filepath.Dir(exe))
	}()

	// Periodically sample transfer counters to accumulate per-client usage
	// (surviving restarts) and enforce download quotas / expiry.
	accountCtx, accountCancel := context.WithCancel(context.Background())
	defer accountCancel()
	go svc.StartAccounting(accountCtx)

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

// quietPaths are polled frequently by the UI; logging their (successful) reads
// would flood the log on an idle panel, so we skip them unless they error.
var quietPaths = map[string]bool{
	"/api/overview": true, "/api/clients": true, "/api/system": true,
	"/api/health": true, "/api/me": true,
}

// logRequests is a tiny access-log middleware. It records the status code so it
// can stay quiet for the chatty poll/static reads but still surface errors.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status < 400 && (quietPaths[r.URL.Path] || strings.HasPrefix(r.URL.Path, "/assets/")) {
			return
		}
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// gzipMiddleware compresses responses for clients that accept gzip. The big win
// is the embedded JS/CSS bundle (~290KB -> ~95KB); JSON responses shrink too.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

// Drop Content-Length (gzip changes the body size) before headers are flushed.
func (g *gzipResponseWriter) WriteHeader(code int) {
	g.ResponseWriter.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	g.ResponseWriter.Header().Del("Content-Length")
	return g.gz.Write(b)
}
