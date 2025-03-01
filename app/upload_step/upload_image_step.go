package upload_step

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log/slog"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/serisow/lesocle/pipeline_type"
)

type UploadImageStepImpl struct {
    PipelineStep pipeline_type.PipelineStep
    Logger       *slog.Logger
}

// FileInfo structure for output that matches Drupal's format
type FileInfo struct {
    FileID    int64  `json:"file_id"`
    URI       string `json:"uri"`
    URL       string `json:"url"`
    MimeType  string `json:"mime_type"`
    Filename  string `json:"filename"`
    Size      int64  `json:"size"`
    Timestamp int64  `json:"timestamp"`
}

func (s *UploadImageStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    s.Logger.Info("Executing upload image step", 
        slog.String("step_id", s.PipelineStep.ID),
        slog.String("uuid", s.PipelineStep.UUID))

    // Validate image configuration is available
    if s.PipelineStep.UploadImageConfig == nil {
        return fmt.Errorf("missing upload image configuration")
    }

    config := s.PipelineStep.UploadImageConfig
    
    // Validate required fields
    if config.FileURL == "" {
        return fmt.Errorf("image file URL is missing in configuration")
    }

    s.Logger.Debug("Image file details", 
        slog.String("url", config.FileURL),
        slog.String("name", config.FileName),
        slog.String("mime", config.FileMime))

    // Download the image to a local file
    localFilePath, err := s.downloadImage(ctx, config)
    if err != nil {
        return fmt.Errorf("failed to download image: %w", err)
    }

    // Get file size
    fileInfo, err := os.Stat(localFilePath)
    if err != nil {
        return fmt.Errorf("failed to get file info: %w", err)
    }

    // Create file info structure for the context
    fileInfoData := FileInfo{
        FileID:    config.FileID,
        URI:       localFilePath,
        URL:       config.FileURL,
        MimeType:  config.FileMime,
        Filename:  config.FileName,
        Size:      fileInfo.Size(),
        Timestamp: time.Now().Unix(),
    }

    // Convert to JSON string for consistent output format
    resultJSON, err := json.Marshal(fileInfoData)
    if err != nil {
        return fmt.Errorf("error marshaling file info: %w", err)
    }

    // Store the result in the context with the step output key
    pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, string(resultJSON))

    s.Logger.Info("Successfully processed image file", 
        slog.String("local_path", localFilePath),
        slog.Int64("size", fileInfo.Size()))

    return nil
}

func (s *UploadImageStepImpl) downloadImage(ctx context.Context, config *pipeline_type.UploadImageConfig) (string, error) {
    // Create directory for downloaded images
    dir := filepath.Join("storage", "pipeline", "images", time.Now().Format("2006-01"))
    if err := os.MkdirAll(dir, 0755); err != nil {
        return "", fmt.Errorf("failed to create directory: %w", err)
    }

    // Generate filename for the downloaded image
    filename := fmt.Sprintf("image_%d_%s", time.Now().UnixNano(), filepath.Base(config.FileName))
    if filename == "" {
        filename = fmt.Sprintf("image_%d.jpg", time.Now().UnixNano())
    }
    outputPath := filepath.Join(dir, filename)

    // Create HTTP client with timeout
    client := &http.Client{
        Timeout: 30 * time.Second,
    }

    // Download the image
    s.Logger.Debug("Downloading image", 
        slog.String("url", config.FileURL), 
        slog.String("to", outputPath))

    req, err := http.NewRequestWithContext(ctx, "GET", config.FileURL, nil)
    if err != nil {
        return "", fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("failed to download image: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("failed to download image, status: %d", resp.StatusCode)
    }

    // Create output file
    file, err := os.Create(outputPath)
    if err != nil {
        return "", fmt.Errorf("failed to create output file: %w", err)
    }
    defer file.Close()

    // Copy the content
    _, err = io.Copy(file, resp.Body)
    if err != nil {
        return "", fmt.Errorf("failed to save image data: %w", err)
    }

    s.Logger.Info("Successfully downloaded image", slog.String("path", outputPath))
    return outputPath, nil
}

func (s *UploadImageStepImpl) GetType() string {
    return "upload_image_step"
}