package main

import (
	"context"
	"database/sql"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/handler"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"
	"github.com/trishtzy/warren/migrations"
	"github.com/trishtzy/warren/templates"
)

func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgresql://rabbithole:rabbithole@127.0.0.1:5433/rabbithole?sslmode=disable"
	}

	ctx := context.Background()

	// Run goose migrations before opening the connection pool.
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fatal("unable to open database for migrations", "error", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		fatal("goose set dialect", "error", err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		fatal("goose migrations failed", "error", err)
	}
	version, err := goose.GetDBVersion(sqlDB)
	if err != nil {
		fatal("unable to get migration version", "error", err)
	}
	if err := sqlDB.Close(); err != nil {
		fatal("unable to close migration db", "error", err)
	}
	slog.Info("migrations applied", "version", version)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fatal("unable to connect to database", "error", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fatal("unable to ping database", "error", err)
	}
	slog.Info("connected to database")

	queries := db.New(pool)
	authService := service.NewAuthService(queries)
	postStore := service.NewPgPostStore(queries, pool)
	postService := service.NewPostService(postStore)

	// Parse each page template individually with the layout to avoid
	// "content" block collisions from ParseGlob merging all definitions.
	tmpl := make(handler.Templates)
	layoutBytes, err := fs.ReadFile(templates.FS, "layout.html")
	if err != nil {
		fatal("unable to read layout template", "error", err)
	}
	entries, err := fs.ReadDir(templates.FS, ".")
	if err != nil {
		fatal("unable to read template dir", "error", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".html") || name == "layout.html" {
			continue
		}
		pageBytes, err := fs.ReadFile(templates.FS, name)
		if err != nil {
			fatal("unable to read template", "name", name, "error", err)
		}
		t, err := template.New(name).Parse(string(layoutBytes))
		if err != nil {
			fatal("unable to parse layout", "name", name, "error", err)
		}
		if _, err := t.Parse(string(pageBytes)); err != nil {
			fatal("unable to parse template", "name", name, "error", err)
		}
		tmpl[name] = t
	}

	authHandler := handler.NewAuthHandler(authService, queries, tmpl)
	postHandler := handler.NewPostHandler(postService, queries, tmpl)

	mux := http.NewServeMux()
	authHandler.RegisterRoutes(mux)
	postHandler.RegisterRoutes(mux)

	// Wrap the entire mux with middleware.
	// CSRF runs first (outermost), then Auth injects agent info.
	wrappedMux := middleware.CSRF(middleware.Auth(queries)(mux))

	addr := ":" + port
	server := &http.Server{
		Addr:         addr,
		Handler:      wrappedMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("server error", "error", err)
		}
	}()

	<-done
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		fatal("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}
