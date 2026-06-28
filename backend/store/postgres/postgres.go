package postgres

import (
	"context"
	"fmt"
	"log"

	"stemsorter/backend/store"
)

// PostgresStore implements the Store interface using PostgreSQL.
type PostgresStore struct {
	// In a real application, this would be a pgx connection pool.
	dsn string
}

// New creates a new PostgresStore.
func New(dsn string) (store.Store, error) {
	log.Printf("Initializing PostgresStore with DSN: %s", dsn)
	return &PostgresStore{
		dsn: dsn,
	}, nil
}

// Ping verifies the connection to the database.
func (s *PostgresStore) Ping(ctx context.Context, tenantID string) error {
	// In a real application, we would set the tenant_id for RLS here before querying.
	// e.g., tx.Exec(ctx, "SET app.tenant_id = $1", tenantID)
	log.Printf("[Postgres] Pinging for tenant: %s", tenantID)

	if tenantID == "" {
		return fmt.Errorf("tenantID is required")
	}

	return nil
}

// Close closes the underlying storage connection(s).
func (s *PostgresStore) Close() error {
	log.Println("Closing PostgresStore")
	return nil
}
