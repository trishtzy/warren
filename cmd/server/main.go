package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/handler"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"
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
	layoutFile := "templates/layout.html"
	pages, err := filepath.Glob("templates/*.html")
	if err != nil {
		log.Fatalf("unable to glob templates: %v", err)
	}
	for _, page := range pages {
		if page == layoutFile {
			continue
		}
		name := filepath.Base(page)
		t, err := template.ParseFiles(layoutFile, page)
		if err != nil {
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
