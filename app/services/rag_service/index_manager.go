package rag_service

import (
    "context"
    "fmt"
    "log/slog"
    "math"

    "github.com/jackc/pgx/v5/pgxpool"
)

// IndexManager handles vector index operations
type IndexManager struct {
    db     *pgxpool.Pool
    logger *slog.Logger
}

func NewIndexManager(db *pgxpool.Pool, logger *slog.Logger) *IndexManager {
    return &IndexManager{
        db:     db,
        logger: logger,
    }
}

// CreateOrUpdateIndex creates or updates the vector index
func (im *IndexManager) CreateOrUpdateIndex(ctx context.Context) error {
    // Count total documents to calculate optimal number of lists
    var count int
    err := im.db.QueryRow(ctx, "SELECT COUNT(*) FROM documents").Scan(&count)
    if err != nil {
        return fmt.Errorf("failed to count documents: %w", err)
    }

    // Calculate optimal number of lists (sqrt of document count)
    lists := int(math.Sqrt(float64(count)))
    if lists < 100 {
        lists = 100 // minimum number of lists
    }

    // Drop existing index if it exists
    _, err = im.db.Exec(ctx, "DROP INDEX IF EXISTS idx_documents_embedding")
    if err != nil {
        return fmt.Errorf("failed to drop existing index: %w", err)
    }

    // Create new index
    createIndexSQL := fmt.Sprintf(`
        CREATE INDEX idx_documents_embedding 
        ON documents 
        USING ivfflat (embedding vector_cosine_ops)
        WITH (lists = %d)
    `, lists)

    _, err = im.db.Exec(ctx, createIndexSQL)
    if err != nil {
        return fmt.Errorf("failed to create index: %w", err)
    }

    im.logger.Info("Vector index created/updated successfully",
        slog.Int("document_count", count),
        slog.Int("list_count", lists))

    return nil
}

// ReindexIfNeeded checks if reindexing is needed and performs it
func (im *IndexManager) ReindexIfNeeded(ctx context.Context) error {
    // Check current index lists count
    var currentLists int
    err := im.db.QueryRow(ctx, `
        SELECT reloptions[1]::text::int
        FROM pg_class c
        LEFT JOIN pg_index i ON c.oid = i.indexrelid
        WHERE c.relname = 'idx_documents_embedding'
        AND reloptions IS NOT NULL
    `).Scan(&currentLists)

    if err != nil {
        // Index doesn't exist or other error
        return im.CreateOrUpdateIndex(ctx)
    }

    // Count documents
    var count int
    err = im.db.QueryRow(ctx, "SELECT COUNT(*) FROM documents").Scan(&count)
    if err != nil {
        return fmt.Errorf("failed to count documents: %w", err)
    }

    optimalLists := int(math.Sqrt(float64(count)))
    if optimalLists < 100 {
        optimalLists = 100
    }

    // If current lists count is significantly different from optimal, rebuild index
    if math.Abs(float64(currentLists-optimalLists)) > float64(optimalLists)*0.5 {
        im.logger.Info("Rebuilding vector index due to significant size change",
            slog.Int("current_lists", currentLists),
            slog.Int("optimal_lists", optimalLists))
        return im.CreateOrUpdateIndex(ctx)
    }

    return nil
}