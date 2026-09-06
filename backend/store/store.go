package store

import (
	"context"
)

// Store is the backend-neutral storage interface that the pipeline and API depend on.
// Note: no query may assume cross-tenant visibility.
type Store interface {
	// Add other methods here based on future requirements (e.g., GetBlob, PutBlob, ListFiles)

	// Close closes the underlying storage connection(s).
	Close() error

	// Ping verifies the connection to the database is still alive.
	Ping(ctx context.Context, tenantID string) error
}
