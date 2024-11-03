package action_service

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log/slog"
    "mime/multipart"
    "net/http"
    "path/filepath"
    "strings"

    "github.com/serisow/lesocle/pipeline_type"
)

type DocumentFetchActionService struct {
    logger     *slog.Logger
    httpClient *http.Client
}

func NewDocumentFetchActionService(logger *slog.Logger) *DocumentFetchActionService {
    return &DocumentFetchActionService{
        logger:     logger,
        httpClient: &http.Client{},
    }
}

func (s *DocumentFetchActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
        return "", fmt.Errorf("missing action configuration for DocumentFetchAction")
    }

    config := step.ActionDetails.Configuration
    ragServiceURL, ok := config["rag_service_url"].(string)
    if !ok {
        return "", fmt.Errorf("rag_service_url not found in config")
    }
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
        docAbsPath := fmt.Sprintf("%s%s", drupalURL, doc.URI);
        fileResp, err := s.httpClient.Get(docAbsPath)
        if err != nil {
            s.logger.Error("Failed to download file from Drupal",
                slog.String("filename", doc.Filename),
                slog.String("uri", doc.URI),
                slog.String("error", err.Error()))
            continue
        }
        
        if fileResp.StatusCode != http.StatusOK {
            s.logger.Error("Failed to download file from Drupal - non-200 status",
                slog.String("filename", doc.Filename),
                slog.String("uri", doc.URI),
                slog.Int("status", fileResp.StatusCode))
            fileResp.Body.Close()
            continue
        }

        // Create form data for ragone service
        form := &bytes.Buffer{}
        writer := multipart.NewWriter(form)
        
        fw, err := writer.CreateFormFile("file", filepath.Base(doc.Filename))
        if err != nil {
            s.logger.Error("Failed to create form file",
                slog.String("filename", doc.Filename),
                slog.String("error", err.Error()))
            fileResp.Body.Close()
            continue
        }
        
        // Copy file content to form
        written, err := io.Copy(fw, fileResp.Body)
        fileResp.Body.Close() // Close file response body after copy

        if err != nil {
            s.logger.Error("Failed to copy file content",
                slog.String("filename", doc.Filename),
                slog.String("error", err.Error()))
            continue
        }

        if written == 0 {
            s.logger.Error("No content copied from file",
                slog.String("filename", doc.Filename))
            continue
        }
        
        writer.Close()

        // Create request to ragone service
        uploadURL := fmt.Sprintf("%s/api/v1/upload", strings.TrimRight(ragServiceURL, "/"))
        req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, form)
        if err != nil {
            s.logger.Error("Failed to create request",
                slog.String("filename", doc.Filename),
                slog.String("error", err.Error()))
            continue
        }

        req.Header.Set("Content-Type", writer.FormDataContentType())

        // Send request to ragone
        resp, err := s.httpClient.Do(req)
        if err != nil {
            s.logger.Error("Failed to upload to RAG service",
                slog.String("filename", doc.Filename),
                slog.String("error", err.Error()))
            continue
        }

        // Parse ragone response
        var result struct {
            Message    string `json:"message"`
            DocumentID int    `json:"documentID"`
        }

        if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
            s.logger.Error("Failed to decode RAG service response",
                slog.String("filename", doc.Filename),
                slog.String("error", err.Error()))
            resp.Body.Close()
            continue
        }
        resp.Body.Close()

        results = append(results, map[string]interface{}{
            "mid":         doc.MID,
            "filename":    doc.Filename,
            "document_id": result.DocumentID,
            "status":     "indexed",
        })

        s.logger.Info("Successfully indexed document",
            slog.String("filename", doc.Filename),
            slog.Int("document_id", result.DocumentID))
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