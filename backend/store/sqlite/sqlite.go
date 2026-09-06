package sqlite

import (
	"context"
	"fmt"
	"log"
	"sync"

	"stemsorter/backend/store"
)

// SQLiteStore implements the Store interface using SQLite-per-tenant.
type SQLiteStore struct {
	baseDir string

	mu          sync.Mutex
	// LRU map in a real implementation.
	connections map[string]interface{} // map[tenantID]*sql.DB
}

// New creates a new SQLiteStore.
func New(baseDir string) (store.Store, error) {
	log.Printf("Initializing SQLiteStore with base directory: %s", baseDir)
	return &SQLiteStore{
		baseDir:     baseDir,
		connections: make(map[string]interface{}),
	}, nil
}

// Ping verifies the connection to the database.
func (s *SQLiteStore) Ping(ctx context.Context, tenantID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[SQLite] Pinging for tenant: %s", tenantID)

	if tenantID == "" {
		return fmt.Errorf("tenantID is required")
	}

	// Simulated connection opening
	if _, ok := s.connections[tenantID]; !ok {
		log.Printf("[SQLite] Opening connection for tenant: %s", tenantID)
		s.connections[tenantID] = struct{}{} // Dummy value
	}

	return nil
}

// Close closes the underlying storage connection(s).
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Println("Closing SQLiteStore")

	// Close all tenant connections here
	s.connections = make(map[string]interface{})

	return nil
}
