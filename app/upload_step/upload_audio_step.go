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

type UploadAudioStepImpl struct {
    PipelineStep pipeline_type.PipelineStep
    Logger       *slog.Logger
}

// AudioFileInfo structure for output that matches Drupal's format
type AudioFileInfo struct {
    FileID     int64     `json:"file_id"`
    URI        string  `json:"uri"`
    URL        string  `json:"url"`
    MimeType   string  `json:"mime_type"`
    Filename   string  `json:"filename"`
    Size       int64   `json:"size"`
    Duration   float64 `json:"duration"`
    Timestamp  int64   `json:"timestamp"`
}

func (s *UploadAudioStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    s.Logger.Info("Executing upload audio step", 
        slog.String("step_id", s.PipelineStep.ID),
        slog.String("uuid", s.PipelineStep.UUID))

    // Validate audio configuration is available
    if s.PipelineStep.UploadAudioConfig == nil {
        return fmt.Errorf("missing upload audio configuration")
    }

    config := s.PipelineStep.UploadAudioConfig
    
    // Validate required fields
    if config.FileURL == "" {
        return fmt.Errorf("audio file URL is missing in configuration")
    }

    s.Logger.Debug("Audio file details", 
        slog.String("url", config.FileURL),
        slog.String("name", config.FileName),
        slog.String("mime", config.FileMime),
        slog.Float64("duration", config.FileDuration))

    // Download the audio to a local file
    localFilePath, err := s.downloadAudio(ctx, config)
    if err != nil {
        return fmt.Errorf("failed to download audio: %w", err)
    }

    // Get file size
    fileInfo, err := os.Stat(localFilePath)
    if err != nil {
        return fmt.Errorf("failed to get file info: %w", err)
    }

    // Create file info structure for the context
    fileInfoData := AudioFileInfo{
        FileID:    config.FileID,
        URI:       localFilePath,
        URL:       config.FileURL,
        MimeType:  config.FileMime,
        Filename:  config.FileName,
        Size:      fileInfo.Size(),
        Duration:  config.FileDuration,
        Timestamp: time.Now().Unix(),
    }

    // Convert to JSON string for consistent output format
    resultJSON, err := json.Marshal(fileInfoData)
    if err != nil {
        return fmt.Errorf("error marshaling file info: %w", err)
    }

    // Store the result in the context with the step output key
    pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, string(resultJSON))

    s.Logger.Info("Successfully processed audio file", 
        slog.String("local_path", localFilePath),
        slog.Int64("size", fileInfo.Size()),
        slog.Float64("duration", config.FileDuration))

    return nil
}

func (s *UploadAudioStepImpl) downloadAudio(ctx context.Context, config *pipeline_type.UploadAudioConfig) (string, error) {
    // Create directory for downloaded audio files
    dir := filepath.Join("storage", "pipeline", "audio", time.Now().Format("2006-01"))
    if err := os.MkdirAll(dir, 0755); err != nil {
        return "", fmt.Errorf("failed to create directory: %w", err)
    }

    // Generate filename for the downloaded audio
    filename := fmt.Sprintf("audio_%d_%s", time.Now().UnixNano(), filepath.Base(config.FileName))
    if filename == "" {
        filename = fmt.Sprintf("audio_%d.mp3", time.Now().UnixNano())
    }
    outputPath := filepath.Join(dir, filename)

    // Create HTTP client with timeout
    client := &http.Client{
        Timeout: 60 * time.Second, // Longer timeout for audio files
    }

    // Download the audio
    s.Logger.Debug("Downloading audio", 
        slog.String("url", config.FileURL), 
        slog.String("to", outputPath))

    req, err := http.NewRequestWithContext(ctx, "GET", config.FileURL, nil)
    if err != nil {
        return "", fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("failed to download audio: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("failed to download audio, status: %d", resp.StatusCode)
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
        return "", fmt.Errorf("failed to save audio data: %w", err)
    }

    s.Logger.Info("Successfully downloaded audio", slog.String("path", outputPath))
    return outputPath, nil
}

func (s *UploadAudioStepImpl) GetType() string {
    return "upload_audio_step"
}