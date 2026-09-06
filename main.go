package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stemsorter/backend/api"
	"stemsorter/backend/store"
	"stemsorter/backend/store/postgres"
	"stemsorter/backend/store/sqlite"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	storeType := flag.String("store", "sqlite", "Storage backend to use (sqlite or postgres)")
	dbConn := flag.String("db", "data", "Database connection string or directory")
	port := flag.String("port", "8080", "Port to listen on")
	flag.Parse()

	log.Printf("Starting Stem Sorter Platform...")

	// 1. Initialize Storage
	var s store.Store
	var err error

	if *storeType == "postgres" {
		s, err = postgres.New(*dbConn)
	} else {
		s, err = sqlite.New(*dbConn)
	}

	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer s.Close()

	// 2. Setup API
	apiServer := api.New(s)
	apiHandler := apiServer.Routes()

	// 3. Setup Frontend Embedded Files
	// Strip the "frontend/dist" prefix from the embedded filesystem
	subFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem: %v", err)
	}
	fileServer := http.FileServer(http.FS(subFS))

	// 4. Main Router
	mux := http.NewServeMux()

	// Mount API routes under /api or directly (handled by apiHandler)
	mux.Handle("/t/", apiHandler) // tenant routes

	// Fallback to frontend for all other routes (SPA routing)
	mux.Handle("/", fileServer)

	server := &http.Server{
		Addr:    ":" + *port,
		Handler: mux,
	}

	// 5. Start Server with Graceful Shutdown
	go func() {
		log.Printf("Listening on http://localhost:%s\n", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server exited gracefully.")
}
