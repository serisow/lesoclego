package rag_service

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serisow/lesocle/pipeline_type"
)

var mimeTypes = map[string]string{
    ".pdf":  "application/pdf",
    ".doc":  "application/msword",
    ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}

type Processor struct {
    db        *pgxpool.Pool
    logger    *slog.Logger
    extractor *DocumentExtractor
}

func NewProcessor(db *pgxpool.Pool, logger *slog.Logger) *Processor {
    return &Processor{
        db:        db,
        logger:    logger,
        extractor: NewDocumentExtractor(logger),
    }
}

func (p *Processor) ProcessDocument(ctx context.Context, filename string, content []byte) (*pipeline_type.RAGResponse, error) {
    ext := filepath.Ext(filename)
    metadata := pipeline_type.DocumentMetadata{
        ContentType: getMimeType(ext),
    }
    
    // Extract text from document
    extractStart := time.Now()
    var text string
    var err error
    
    switch ext {
    case ".pdf":
        text, err = p.extractor.ExtractTextFromPDF(content)
    case ".doc", ".docx":
        text, err = p.extractor.ExtractTextFromWord(content)
    default:
        return nil, fmt.Errorf("unsupported file type: %s", ext)
    }

    if err != nil {
        p.logger.Error("Text extraction failed",
            slog.String("filename", filename),
            slog.String("error", err.Error()))
            
        return &pipeline_type.RAGResponse{
            Message: "Failed to extract text from document",
            Status:  "failed",
            Error:   err.Error(),
            Metadata: metadata,
        }, nil
    }

    metadata.ProcessingStats.ExtractionTime = time.Since(extractStart).Seconds()
    metadata.WordCount = len(strings.Fields(text))
    if len(text) > 250 {
        metadata.ContentPreview = text[:250] + "..."
    } else {
        metadata.ContentPreview = text
    }

    // Generate embedding
    embedStart := time.Now()
    embedding, tokenCount, err := GetEmbeddingWithTokenCount(text)
    if err != nil {
        return nil, fmt.Errorf("failed to generate embedding: %w", err)
    }
    metadata.ProcessingStats.EmbeddingTime = time.Since(embedStart).Seconds()
    metadata.TokenCount = tokenCount

    // Store in database
    var documentID int
    query := `INSERT INTO documents (filename, content, embedding) VALUES ($1, $2, $3) RETURNING id`
    err = p.db.QueryRow(ctx, query, filename, text, embedding).Scan(&documentID)
    if err != nil {
        return nil, fmt.Errorf("failed to store document: %w", err)
    }

    return &pipeline_type.RAGResponse{
        Message:    "Document processed successfully",
        DocumentID: documentID,
        Status:     "indexed",
        Metadata:   metadata,
    }, nil
}

func getMimeType(ext string) string {
    if mime, ok := mimeTypes[strings.ToLower(ext)]; ok {
        return mime
    }
    return "application/octet-stream" // default mime type
}