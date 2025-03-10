package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	envConfig "github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/pipeline_type"
)

const (
	VideoGenerationServiceName = "video_generation"
)

// VideoGenerationActionService is the main service for video generation
type VideoGenerationActionService struct {
	logger         *slog.Logger
	fileManager    FileManager
	ffmpegExecutor FFmpegExecutor
	textProcessor  TextProcessor
}

// NewVideoGenerationActionService creates a new video generation service
func NewVideoGenerationActionService(logger *slog.Logger) *VideoGenerationActionService {
	// Create a concrete implementation of the TextProcessor interface
	textProcessor := NewTextProcessor()
	return &VideoGenerationActionService{
		logger:         logger,
		fileManager:    NewFileManager(logger),
		ffmpegExecutor: NewFFmpegExecutor(logger, textProcessor),
		textProcessor:  textProcessor,
	}
}

// CanHandle checks if this service can handle the given action service
func (s *VideoGenerationActionService) CanHandle(actionService string) bool {
	return actionService == VideoGenerationServiceName
}

// Execute processes a video generation action
func (s *VideoGenerationActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for VideoGenerationAction")
	}

	s.logger.Info("Starting video generation",
		slog.String("step_id", step.ID),
		slog.String("step_uuid", step.UUID))

	// Find all images with output type "featured_image"
	imageFiles, err := s.fileManager.FindFilesByOutputType(ctx, pipelineContext, "featured_image")
	if err != nil {
		return "", fmt.Errorf("image files not found: %w", err)
	}

	// Find audio file with output type "audio_content"
	audioFileInfo, err := s.fileManager.FindFileByOutputType(ctx, pipelineContext, "audio_content")
	if err != nil {
		return "", fmt.Errorf("audio file information not found: %w", err)
	}

	s.logger.Debug("Found required files",
		slog.Int("image_count", len(imageFiles)),
		slog.String("audio_uri", audioFileInfo.URI))

	// Get file paths from URIs
	imagePaths := make([]string, len(imageFiles))
	for i, imageFile := range imageFiles {
		imagePaths[i] = s.fileManager.URIToFilePath(imageFile.URI)

		// Verify file exists
		if _, err := os.Stat(imagePaths[i]); os.IsNotExist(err) {
			return "", fmt.Errorf("image file not found at path: %s", imagePaths[i])
		}

	}

	audioFilePath := s.fileManager.URIToFilePath(audioFileInfo.URI)
	if _, err := os.Stat(audioFilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file not found at path: %s", audioFilePath)
	}

	// Prepare output directory and filename
	outputDir := filepath.Join("storage", "pipeline", "videos", time.Now().Format("2006-01"))
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get configuration parameters
	config := step.ActionDetails.Configuration
	outputFormat := getStringValue(config, "output_format", "mp4")

	// Extract file ID for the URL
	fileID := fmt.Sprintf("%d", time.Now().UnixNano())

	filename := fmt.Sprintf("video_%s.%s", fileID, outputFormat)
	
	outputPath := filepath.Join(outputDir, filename)

	// Get video quality settings
	videoQuality := getStringValue(config, "video_quality", "medium")
	resolution := s.ffmpegExecutor.GetResolution(videoQuality)

	// Get audio duration
	audioDuration, err := s.ffmpegExecutor.GetAudioDuration(audioFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Get transition duration from config
	transitionDurationStr := getStringValue(config, "transition_duration", "1")
	transitionDuration, err := strconv.ParseFloat(transitionDurationStr, 64)
	if err != nil {
		// Default to 1 second if parsing fails
		transitionDuration = 1.0
		s.logger.Warn("Failed to parse transition_duration, using default",
			slog.String("value", transitionDurationStr),
			slog.Float64("default", transitionDuration))
	}

	// Calculate durations for each image
	imageDurations := s.ffmpegExecutor.CalculateImageDurations(imageFiles, audioDuration, transitionDuration)

	// Prepare video generation parameters
	videoParams := VideoParams{
		ImageFiles:         imageFiles,
		AudioFile:          audioFileInfo,
		OutputPath:         outputPath,
		Resolution:         resolution,
		TransitionType:     getStringValue(config, "transition_type", "fade"),
		TransitionDuration: transitionDuration,
		ImageDurations:     imageDurations,
		Config:             config,
		PipelineContext:    pipelineContext,
	}

	// Create video using FFmpeg with text overlays
	err = s.ffmpegExecutor.CreateMultiImageVideo(videoParams)
	if err != nil {
		return "", fmt.Errorf("failed to create video: %w", err)
	}

	// Get file size
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to get video file info: %w", err)
	}

	// Create response with slide information
	slides := make([]map[string]interface{}, len(imageFiles))
	for i, imageFile := range imageFiles {
		slideInfo := map[string]interface{}{
			"file_id":  imageFile.FileID,
			"duration": imageDurations[i],
			"step_key": imageFile.StepKey,
		}

		// Include text blocks info in response
		if len(imageFile.TextBlocks) > 0 {
			textBlocksInfo := make([]map[string]interface{}, 0)
			for _, block := range imageFile.TextBlocks {
				if block.Enabled {
					blockInfo := map[string]interface{}{
						"id":         block.ID,
						"text":       block.Text,
						"position":   block.Position,
						"font_size":  block.FontSize,
						"font_color": block.FontColor,
					}
					if block.BackgroundColor != "" {
						blockInfo["background_color"] = block.BackgroundColor
					}
					textBlocksInfo = append(textBlocksInfo, blockInfo)
				}
			}
			slideInfo["text_blocks"] = textBlocksInfo
		}

		slides[i] = slideInfo
	}

	// Load config to get base URL
	cfg := envConfig.Load()

	// Create absolute download URL
	absoluteDownloadURL := fmt.Sprintf("%s/api/videos/%s", cfg.ServiceBaseURL, fileID)

	// Create response with download URL
	result := map[string]interface{}{
		"file_id":      fileID,
		"uri":          outputPath,
		"url":          fmt.Sprintf("/storage/pipeline/videos/%s/%s", time.Now().Format("2006-01"), filename),
		"download_url": absoluteDownloadURL,
		"mime_type":    fmt.Sprintf("video/%s", outputFormat),
		"filename":     filename,
		"duration":     audioDuration,
		"size":         fileInfo.Size(),
		"timestamp":    time.Now().Unix(),
		"slides":       slides,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("error marshaling result: %w", err)
	}

	s.logger.Info("Video generation completed successfully",
		slog.String("output_path", outputPath),
		slog.Float64("duration", audioDuration),
		slog.Int("slide_count", len(imageFiles)))

	return string(resultJSON), nil
}
