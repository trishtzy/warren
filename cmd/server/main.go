package main

import (
	"context"
	"database/sql"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/handler"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"
	"github.com/trishtzy/warren/migrations"
	"github.com/trishtzy/warren/templates"
)

func main() {
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
		log.Fatalf("unable to open database for migrations: %v", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("goose set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		log.Fatalf("goose migrations failed: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		log.Fatalf("unable to close migration db: %v", err)
	}
	log.Println("migrations applied")

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("unable to ping database: %v", err)
	}
	log.Println("connected to database")

	queries := db.New(pool)
	authService := service.NewAuthService(queries)
	postStore := service.NewPgPostStore(queries, pool)
	postService := service.NewPostService(postStore)

	// Parse each page template individually with the layout to avoid
	// "content" block collisions from ParseGlob merging all definitions.
	tmpl := make(handler.Templates)
	layoutBytes, err := fs.ReadFile(templates.FS, "layout.html")
	if err != nil {
		log.Fatalf("unable to read layout template: %v", err)
	}
	entries, err := fs.ReadDir(templates.FS, ".")
	if err != nil {
		log.Fatalf("unable to read template dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".html") || name == "layout.html" {
			continue
		}
		pageBytes, err := fs.ReadFile(templates.FS, name)
		if err != nil {
			log.Fatalf("unable to read template %s: %v", name, err)
		}
		t, err := template.New(name).Parse(string(layoutBytes))
		if err != nil {
			log.Fatalf("unable to parse layout for %s: %v", name, err)
		}
		if _, err := t.Parse(string(pageBytes)); err != nil {
			log.Fatalf("unable to parse template %s: %v", name, err)
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
		log.Printf("listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-done
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}
