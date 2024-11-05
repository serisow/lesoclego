package pipeline_type

import "github.com/pgvector/pgvector-go"


type ProcessingStats struct {
    ExtractionTime float64 `json:"extraction_time"`
    EmbeddingTime  float64 `json:"embedding_time"`
}


type Document struct {
    ID        int
    Filename  string
    Content   string
    Embedding *pgvector.Vector
}

type DocumentMetadata struct {
    WordCount      int            `json:"word_count"`
    TokenCount     int            `json:"token_count"`
    ContentPreview string         `json:"content_preview"`
    ContentType    string         `json:"content_type"`
    ProcessingStats ProcessingStats `json:"processing_stats"`
}

type RAGResponse struct {
    Message    string          `json:"message"`
    DocumentID int             `json:"documentID"`
    Metadata   DocumentMetadata `json:"metadata"`
	Error      string          `json:"error"`
    Status     string          `json:"status"`
}