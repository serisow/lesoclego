package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serisow/lesocle/services/rag_service"
)

// SearchConfig represents the configuration for document search
type SearchConfig struct {
    SimilarityThreshold string  `json:"similarity_threshold"`
    MaxResults          string  `json:"max_results"`
	SimilarityMetric   string  `json:"similarity_metric"`
    IncludeMetadata     int     `json:"include_metadata"` 
    MinWordCount        string  `json:"min_word_count"`
	ExcludeAlreadyUsed  int     `json:"exclude_already_used"`
}

// SearchRequest represents the incoming search request
type SearchRequest struct {
	Query  string       `json:"query"`
	Config SearchConfig `json:"config"`
}

// SearchResult represents a single document search result
type SearchResult struct {
	DocumentID      int                    `json:"document_id"`
	Content        string                 `json:"content"`
	SimilarityScore float64               `json:"similarity_score"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SearchResponse represents the response sent back to Drupal
type SearchResponse struct {
	Documents []SearchResult `json:"documents"`
	Count     int           `json:"count"`
}

// DocumentSearchHandler handles document similarity search requests
type DocumentSearchHandler struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewDocumentSearchHandler creates a new document search handler
func NewDocumentSearchHandler(db *pgxpool.Pool, logger *slog.Logger) *DocumentSearchHandler {
	return &DocumentSearchHandler{
		db:     db,
		logger: logger,
	}
}

// ServeHTTP handles the HTTP request for document search
func (h *DocumentSearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode request body",
			slog.String("error", err.Error()))
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if err := h.ValidateRequest(&req); err != nil {
		h.logger.Error("Invalid request parameters",
			slog.String("error", err.Error()))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get embedding for search query
	embedding, _, err := rag_service.GetEmbeddingWithTokenCount(req.Query)
	if err != nil {
		h.logger.Error("Failed to generate embedding for search query",
			slog.String("error", err.Error()))
		http.Error(w, "Failed to process search query", http.StatusInternalServerError)
		return
	}

	// Build and execute search query
	query := h.BuildSearchQuery(&req, embedding)
	rows, err := h.db.Query(r.Context(), query.Query, query.Args...)
	if err != nil {
		h.logger.Error("Failed to execute search query",
			slog.String("error", err.Error()))
		http.Error(w, "Database query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()


	// Process results
	results := make([]SearchResult, 0)
	for rows.Next() {
		var result SearchResult
		var metadata string
		err := rows.Scan(
			&result.DocumentID,
			&result.Content,
			&result.SimilarityScore,
			&metadata,
		)
		if err != nil {
			h.logger.Error("Failed to scan row",
				slog.String("error", err.Error()))
			continue
		}

		if req.Config.IncludeMetadata == 1 && metadata != "" {
			if err := json.Unmarshal([]byte(metadata), &result.Metadata); err != nil {
				h.logger.Error("Failed to parse metadata",
					slog.String("error", err.Error()),
					slog.Int("document_id", result.DocumentID))
			}
		}

		results = append(results, result)
	}

	response := SearchResponse{
		Documents: results,
		Count:    len(results),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response",
			slog.String("error", err.Error()))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

type QueryBuilder struct {
	Query string
	Args  []interface{}
}

func (h *DocumentSearchHandler) BuildSearchQuery(req *SearchRequest, embedding interface{}) *QueryBuilder {
    qb := &QueryBuilder{
        Args: make([]interface{}, 0),
    }

    // Use CTE for clarity and to allow filtering by similarity score
    qb.Query = `
        WITH scored_documents AS (
            SELECT 
                d.id,
                d.content,
                CASE WHEN $1 = 'cosine' THEN 
                    1 - (d.embedding <=> $2)
                WHEN $1 = 'euclidean' THEN 
                    1 / (1 + (d.embedding <-> $2))
                ELSE
                    d.embedding <#> $2
                END as similarity_score,
                ''::text as metadata
            FROM 
                documents d
            WHERE 1=1
    `
    qb.Args = append(qb.Args, req.Config.SimilarityMetric, embedding)

    // Add minimum word count filter if specified
    minWordCount, _ := strconv.Atoi(req.Config.MinWordCount)
    if minWordCount > 0 {
        qb.Query += " AND array_length(regexp_split_to_array(d.content, '\\s+'), 1) >= $" + fmt.Sprint(len(qb.Args)+1)
        qb.Args = append(qb.Args, minWordCount)
    }

    qb.Query += ")"

    // Now we can filter by similarity_score
    similarityThreshold, _ := strconv.ParseFloat(req.Config.SimilarityThreshold, 64)
    qb.Query += fmt.Sprintf("\nSELECT * FROM scored_documents WHERE similarity_score >= $%d", len(qb.Args)+1)
    qb.Args = append(qb.Args, similarityThreshold)

    // Order by similarity score and limit results
    qb.Query += " ORDER BY similarity_score DESC"
    maxResults, _ := strconv.Atoi(req.Config.MaxResults)
    qb.Query += fmt.Sprintf(" LIMIT $%d", len(qb.Args)+1)
    qb.Args = append(qb.Args, maxResults)

    return qb
}

func (h *DocumentSearchHandler) ValidateRequest(req *SearchRequest) error {
    if req.Query == "" {
        return fmt.Errorf("search query cannot be empty")
    }

    // Convert and validate similarity threshold
    threshold, err := strconv.ParseFloat(req.Config.SimilarityThreshold, 64)
    if err != nil {
        return fmt.Errorf("invalid similarity threshold: %v", err)
    }
    if threshold < 0 || threshold > 1 {
        return fmt.Errorf("similarity threshold must be between 0 and 1")
    }

    // Convert and validate max results
    maxResults, err := strconv.Atoi(req.Config.MaxResults)
    if err != nil {
        return fmt.Errorf("invalid max results: %v", err)
    }
    if maxResults < 1 || maxResults > 50 {
        return fmt.Errorf("max results must be between 1 and 50")
    }

    // Convert and validate min word count
    minWordCount, err := strconv.Atoi(req.Config.MinWordCount)
    if err != nil {
        return fmt.Errorf("invalid min word count: %v", err)
    }
    if minWordCount < 0 {
        return fmt.Errorf("minimum word count cannot be negative")
    }

    switch req.Config.SimilarityMetric {
    case "cosine", "euclidean", "inner_product":
        // Valid metrics
    default:
        return fmt.Errorf("invalid similarity metric: %s", req.Config.SimilarityMetric)
    }

    return nil
}