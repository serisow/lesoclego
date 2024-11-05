package action_service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/services/rag_service"
)

type DocumentFetchActionService struct {
    logger     *slog.Logger
    httpClient *http.Client
    processor *rag_service.Processor

}

func NewDocumentFetchActionService(logger *slog.Logger, processor *rag_service.Processor) *DocumentFetchActionService {
    return &DocumentFetchActionService{
        logger:     logger,
        httpClient: &http.Client{},
        processor: processor,
    }
}

// Modified fetch_document_for_rag.go
func (s *DocumentFetchActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
        return "", fmt.Errorf("missing action configuration for DocumentFetchAction")
    }

    config := step.ActionDetails.Configuration
    drupalURL, ok := config["drupal_url"].(string)
    if !ok {
        return "", fmt.Errorf("drupal_url not found in config")
    }
    batchSize := config["batch_size"].(string)
    statusFilter := config["status_filter"].(string)

    // Fetch documents from Drupal API
    fetchURL := fmt.Sprintf("%s/api/pipeline/documents?batch_size=%s&status=%s", 
        strings.TrimRight(drupalURL, "/"), 
        batchSize, 
        statusFilter,
    )

    req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
    if err != nil {
        return "", fmt.Errorf("failed to create fetch request: %w", err)
    }

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("failed to fetch documents: %w", err)
    }
    defer resp.Body.Close()

    var docsData struct {
        Documents []struct {
            MID      string `json:"mid"`
            Filename string `json:"filename"`
            URI      string `json:"uri"`
            MimeType string `json:"mime_type"`
            Size     int    `json:"size"`
        } `json:"documents"`
        Count     int   `json:"count"`
        Timestamp int64 `json:"timestamp"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&docsData); err != nil {
        return "", fmt.Errorf("error parsing documents response: %w", err)
    }

    // Process each document
    var results []map[string]interface{}
    for _, doc := range docsData.Documents {
        // Download file from Drupal
        docAbsPath := fmt.Sprintf("%s%s", drupalURL, doc.URI)
        fileResp, err := s.httpClient.Get(docAbsPath)

        if err != nil {
            s.logger.Error("Failed to download file from Drupal",
                slog.String("filename", doc.Filename),
                slog.String("uri", doc.URI),
                slog.String("error", err.Error()))
            
            results = append(results, map[string]interface{}{
                "mid":         doc.MID,
                "filename":    doc.Filename,
                "document_id": nil,
                "status":     "failed",
                "metadata":   map[string]interface{}{},
            })
            continue
        }
        
        if fileResp.StatusCode != http.StatusOK {
            s.logger.Error("Failed to download file from Drupal - non-200 status",
                slog.String("filename", doc.Filename),
                slog.String("uri", doc.URI),
                slog.Int("status", fileResp.StatusCode))
            fileResp.Body.Close()
            
            results = append(results, map[string]interface{}{
                "mid":         doc.MID,
                "filename":    doc.Filename,
                "document_id": nil,
                "status":     "failed",
                "metadata":   map[string]interface{}{},
            })
            continue
        }

        // Read file content
        content, err := io.ReadAll(fileResp.Body)
        fileResp.Body.Close()

        if err != nil {
            s.logger.Error("Failed to read file content",
                slog.String("filename", doc.Filename),
                slog.String("error", err.Error()))
            
            results = append(results, map[string]interface{}{
                "mid":         doc.MID,
                "filename":    doc.Filename,
                "document_id": nil,
                "status":     "failed",
                "metadata":   map[string]interface{}{},
            })
            continue
        }

        // Process document directly using the processor
        result, err := s.processor.ProcessDocument(ctx, doc.Filename, content)
        if err != nil {
            s.logger.Error("Failed to process document",
                slog.String("filename", doc.Filename),
                slog.String("error", err.Error()))
            
            results = append(results, map[string]interface{}{
                "mid":         doc.MID,
                "filename":    doc.Filename,
                "document_id": nil,
                "status":     "failed",
                "metadata":   map[string]interface{}{},
            })
            continue
        }

        results = append(results, map[string]interface{}{
            "mid":         doc.MID,
            "filename":    doc.Filename,
            "document_id": result.DocumentID,
            "status":     result.Status,
            "metadata":   result.Metadata,
        })

        if result.Status == "indexed" {
            s.logger.Info("Successfully indexed document",
                slog.String("filename", doc.Filename),
                slog.Int("document_id", result.DocumentID))
        }
    }

    // Return results
    resultJSON, err := json.Marshal(map[string]interface{}{
        "indexed_documents": results,
        "count":            len(results),
    })
    if err != nil {
        return "", fmt.Errorf("error marshaling results: %w", err)
    }

    return string(resultJSON), nil
}

func (s *DocumentFetchActionService) CanHandle(actionService string) bool {
    return actionService == "document_fetch"
}

