package document_search_step

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/davecgh/go-spew/spew"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serisow/lesocle/handlers"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/services/rag_service"
)

type DocumentSearchStepImpl struct {
    PipelineStep pipeline_type.PipelineStep
    DB          *pgxpool.Pool
    Logger      *slog.Logger
}

func (s *DocumentSearchStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    if s.DB == nil {
        return fmt.Errorf("database connection not initialized for document search step")
    }

	spew.Dump(s.PipelineStep)
	
    // Check if SearchSettings and ContentSettings are initialized
    if s.PipelineStep.DocumentSearchSettings == nil {
        s.Logger.Warn("SearchSettings not provided, using defaults")
        s.PipelineStep.DocumentSearchSettings = &pipeline_type.DocumentSearchSettings{
            SimilarityThreshold: "0.8",
            MaxResults:          "5",
            SimilarityMetric:    "cosine",
        }
    }

    if s.PipelineStep.ContentSearchSettings == nil {
        s.Logger.Warn("ContentSettings not provided, using defaults")
        s.PipelineStep.ContentSearchSettings = &pipeline_type.ContentSearchSettings{
            IncludeMetadata:    1,
            MinWordCount:       "0",
            ExcludeAlreadyUsed: 0,
        }
    }

    // Log the configuration for debugging
    s.Logger.Debug("Document search configuration",
        slog.String("search_input", s.PipelineStep.SearchInput),
        slog.Any("search_settings", s.PipelineStep.DocumentSearchSettings),
        slog.Any("content_settings", s.PipelineStep.ContentSearchSettings))



    // Convert pipeline step config to handler's search request format
    searchReq := &handlers.SearchRequest{
        Query: s.PipelineStep.SearchInput,
        Config: handlers.SearchConfig{
            SimilarityThreshold: s.PipelineStep.DocumentSearchSettings.SimilarityThreshold,
            MaxResults:          s.PipelineStep.DocumentSearchSettings.MaxResults,
            SimilarityMetric:    s.PipelineStep.DocumentSearchSettings.SimilarityMetric,
            IncludeMetadata:     s.PipelineStep.ContentSearchSettings.IncludeMetadata,
            MinWordCount:        s.PipelineStep.ContentSearchSettings.MinWordCount,
            ExcludeAlreadyUsed:  s.PipelineStep.ContentSearchSettings.ExcludeAlreadyUsed,
        },
    }

    // Create a handler instance to reuse its functionality
    handler := handlers.NewDocumentSearchHandler(s.DB, s.Logger)

    // Validate the request
    if err := handler.ValidateRequest(searchReq); err != nil {
        return fmt.Errorf("invalid search configuration: %w", err)
    }

    // Get embedding for search query
    embedding, tokenCount, err := rag_service.GetEmbeddingWithTokenCount(searchReq.Query)
    if err != nil {
        return fmt.Errorf("failed to generate embedding for search query: %w", err)
    }

    // Build and execute search query using handler's query builder
    queryBuilder := handler.BuildSearchQuery(searchReq, embedding)
    rows, err := s.DB.Query(ctx, queryBuilder.Query, queryBuilder.Args...)
    if err != nil {
        return fmt.Errorf("failed to execute search query: %w", err)
    }
    defer rows.Close()

    // Process results using handler's result structure
    var results []handlers.SearchResult
    for rows.Next() {
        var result handlers.SearchResult
        var metadata string
        err := rows.Scan(
            &result.DocumentID,
            &result.Content,
            &result.SimilarityScore,
            &metadata,
        )
        if err != nil {
            s.Logger.Error("Failed to scan row",
                slog.String("error", err.Error()))
            continue
        }

        if searchReq.Config.IncludeMetadata == 1 && metadata != "" {
            if err := json.Unmarshal([]byte(metadata), &result.Metadata); err != nil {
                s.Logger.Error("Failed to parse metadata",
                    slog.String("error", err.Error()),
                    slog.Int("document_id", result.DocumentID))
            }
        }

        results = append(results, result)
    }

    // Prepare final result
    finalResult := map[string]interface{}{
        "similar_documents": results,
        "count":            len(results),
        "search_settings": map[string]interface{}{
            "threshold":    searchReq.Config.SimilarityThreshold,
            "metric":      searchReq.Config.SimilarityMetric,
            "max_results": searchReq.Config.MaxResults,
        },
        "metadata": map[string]interface{}{
            "token_count": tokenCount,
            "input_text":  searchReq.Query,
        },
    }

    // Store in pipeline context
    resultJSON, err := json.Marshal(finalResult)
    if err != nil {
        return fmt.Errorf("error marshaling results: %w", err)
    }

    pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, string(resultJSON))
    return nil
}

func (s *DocumentSearchStepImpl) GetType() string {
    return "document_search"
}