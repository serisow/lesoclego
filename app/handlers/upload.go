package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/services/rag_service"
)

type UploadHandler struct {
    db *pgxpool.Pool
	logger    *slog.Logger
    extractor *rag_service.DocumentExtractor
}

func NewUploadHandler(db *pgxpool.Pool, logger *slog.Logger) *UploadHandler {
    return &UploadHandler{
        db:        db,
        logger:    logger,
        extractor: rag_service.NewDocumentExtractor(logger),
    }
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Received file upload request")

    // Parse the incoming multipart form
    err := r.ParseMultipartForm(10 << 20) // 10 MB limit
    if err != nil {
        writeJSONError(w, "Failed to parse multipart form", http.StatusBadRequest)
        return
    }

    // Get the file from the form
    file, header, err := r.FormFile("file")
    if err != nil {
        writeJSONError(w, "Failed to get file from form", http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Read the file into a buffer
    var buf bytes.Buffer
    if _, err := io.Copy(&buf, file); err != nil {
        writeJSONError(w, "Failed to read file", http.StatusInternalServerError)
        return
    }

	h.logger.Debug("Starting text extraction",
	slog.String("filename", header.Filename),
	slog.String("content_type", header.Header.Get("Content-Type")),
	slog.Int64("size", header.Size))

    // Determine file type by extension
    ext := filepath.Ext(header.Filename)
    var text string
	metadata := pipeline_type.DocumentMetadata{
        ContentType: header.Header.Get("Content-Type"),
    }
    
    // Measure extraction time
    extractStart := time.Now()
    
    switch ext {
    case ".pdf":
        text, err = h.extractor.ExtractTextFromPDF(buf.Bytes())
    case ".doc", ".docx":
        text, err = h.extractor.ExtractTextFromWord(buf.Bytes())
    default:
        h.logger.Error("Unsupported file type",
            slog.String("filename", header.Filename),
            slog.String("extension", ext))
        writeJSONError(w, "Unsupported file type", http.StatusBadRequest)
        return
    }

    if err != nil {
        h.logger.Error("Text extraction failed",
            slog.String("filename", header.Filename),
            slog.String("error", err.Error()))
            
        errorResponse := pipeline_type.RAGResponse{
            Message: "Failed to extract text from document",
            Status:  "failed",
            Error:   err.Error(),
            Metadata: pipeline_type.DocumentMetadata{
                ContentType: header.Header.Get("Content-Type"),
                ProcessingStats: pipeline_type.ProcessingStats{
                    ExtractionTime: time.Since(extractStart).Seconds(),
                },
            },
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(errorResponse)
        return
    }

    metadata.ProcessingStats.ExtractionTime = time.Since(extractStart).Seconds()

    // Calculate metadata
    metadata.WordCount = len(strings.Fields(text))
    if len(text) > 250 {
        metadata.ContentPreview = text[:250] + "..."
    } else {
        metadata.ContentPreview = text
    }

    // Generate embedding
    embedStart := time.Now()
    embedding, tokenCount, err := rag_service.GetEmbeddingWithTokenCount(text)
    if err != nil {
        log.Printf("Failed to get embedding: %v", err)
        writeJSONError(w, "Failed to generate embedding", http.StatusInternalServerError)
        return
    }
    metadata.ProcessingStats.EmbeddingTime = time.Since(embedStart).Seconds()
    metadata.TokenCount = tokenCount

    // Create the document model
    doc := pipeline_type.Document{
        Filename:  header.Filename,
        Content:   text,
        Embedding: embedding,
    }

    // Insert the document into the database
    query := `INSERT INTO documents (filename, content, embedding) VALUES ($1, $2, $3) RETURNING id`
    err = h.db.QueryRow(context.Background(), query, doc.Filename, doc.Content, doc.Embedding).Scan(&doc.ID)
    if err != nil {
        log.Printf("Failed to store document: %v", err)
        writeJSONError(w, "Failed to store document", http.StatusInternalServerError)
        return
    }

    // Prepare response
    response := pipeline_type.RAGResponse{
        Message:    "File uploaded and processed successfully",
        DocumentID: doc.ID,
        Metadata:   metadata,
    }

    // Write JSON response
    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(response); err != nil {
        log.Printf("Failed to write response: %v", err)
        writeJSONError(w, "Failed to write response", http.StatusInternalServerError)
        return
    }
}

func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(map[string]string{"error": message})
}