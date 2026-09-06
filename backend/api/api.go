package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"stemsorter/backend/store"
)

type contextKey string

const tenantKey contextKey = "tenant"

// Server handles API routing and dependencies.
type Server struct {
	store store.Store
}

// New creates a new API Server.
func New(s store.Store) *Server {
	return &Server{store: s}
}

// TenantMiddleware extracts the tenant ID from the path prefix /t/{tenant}/ and adds it to the context.
func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/t/") {
			parts := strings.SplitN(path, "/", 4)
			if len(parts) >= 3 {
				tenantID := parts[2]
				ctx := context.WithValue(r.Context(), tenantKey, tenantID)

				// Optional: Strip the tenant prefix for easier downstream routing
				// r.URL.Path = "/" + strings.Join(parts[3:], "/")

				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		http.Error(w, "tenant not specified", http.StatusBadRequest)
	})
}

// HandleAudioStream is a stub for the encrypted range-streaming endpoint.
func (s *Server) HandleAudioStream(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := r.Context().Value(tenantKey).(string)
	if !ok {
		http.Error(w, "tenant context missing", http.StatusInternalServerError)
		return
	}

	trackID := r.URL.Query().Get("id")
	if trackID == "" {
		http.Error(w, "missing track id", http.StatusBadRequest)
		return
	}

	rangeHeader := r.Header.Get("Range")

	log.Printf("[API] Streaming audio for tenant: %s, track: %s, range: %s", tenantID, trackID, rangeHeader)

	// In a real application, this would fetch the streaming AEAD ciphertext,
	// decrypt the requested chunks based on the range, and stream them.

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "bytes")

	if rangeHeader != "" {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	fmt.Fprintf(w, "Stubbed audio stream for track %s (tenant %s)", trackID, tenantID)
}

// Routes returns an HTTP router with API endpoints configured.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// API routes that require a tenant
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/stream", s.HandleAudioStream)
	// Add more routes here...

	// Apply tenant middleware to all /t/ routes
	mux.Handle("/t/", TenantMiddleware(http.StripPrefix("/t", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the tenant ID from the path and forward to apiMux
		// e.g., /t/mytenant/stream -> /stream
		parts := strings.SplitN(r.URL.Path, "/", 3)
		if len(parts) > 2 {
			r.URL.Path = "/" + parts[2]
		} else {
			r.URL.Path = "/"
		}
		apiMux.ServeHTTP(w, r)
	}))))

	return mux
}
