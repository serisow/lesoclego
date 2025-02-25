package action_service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/serisow/lesocle/pipeline_type"
)

const (
	VideoGenerationServiceName = "video_generation"
)

type VideoGenerationActionService struct {
	logger *slog.Logger
}

type FileInfo struct {
	FileID    string `json:"file_id"`
	URI       string `json:"uri"`
	URL       string `json:"url"`
	MimeType  string `json:"mime_type"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Timestamp int64  `json:"timestamp"`
}

func NewVideoGenerationActionService(logger *slog.Logger) *VideoGenerationActionService {
	return &VideoGenerationActionService{
		logger: logger,
	}
}

func (s *VideoGenerationActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for VideoGenerationAction")
	}

	s.logger.Info("Starting video generation",
		slog.String("step_id", step.ID),
		slog.String("step_uuid", step.UUID))
		
	// Check if we have been given the expected file information in the required steps
	if step.RequiredSteps == "" {
		return "", fmt.Errorf("required_steps field is empty, expected image and audio step references")
	}

	// Process required steps to check for data references
	s.logger.Debug("Required steps",
		slog.String("required_steps", step.RequiredSteps))

	// Extract image and audio file information from context using output_type approach
	// This follows the Drupal implementation pattern
	imageFileInfo, err := s.findFileInfo(pipelineContext, "featured_image")
	if err != nil {
		return "", fmt.Errorf("image file information not found: %w", err)
	}

	audioFileInfo, err := s.findFileInfo(pipelineContext, "audio_content")
	if err != nil {
		return "", fmt.Errorf("audio file information not found: %w", err)
	}

	s.logger.Debug("Found required files",
		slog.String("image_uri", imageFileInfo.URI),
		slog.String("audio_uri", audioFileInfo.URI))

	// Get file paths from URIs
	imageFilePath := s.uriToFilePath(imageFileInfo.URI)
	audioFilePath := s.uriToFilePath(audioFileInfo.URI)

	// Verify files exist
	if _, err := os.Stat(imageFilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("image file not found at path: %s", imageFilePath)
	}

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
	outputFormat := "mp4"
	if format, ok := config["output_format"].(string); ok && format != "" {
		outputFormat = format
	}

	filename := fmt.Sprintf("video_%d.%s", time.Now().UnixNano(), outputFormat)
	outputPath := filepath.Join(outputDir, filename)

	// Get video quality settings
	videoQuality := "medium"
	if quality, ok := config["video_quality"].(string); ok && quality != "" {
		videoQuality = quality
	}
	resolution := s.getResolution(videoQuality)

	// Get audio duration
	duration, err := s.getAudioDuration(audioFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Create video using FFmpeg
	err = s.createVideo(imageFilePath, audioFilePath, outputPath, duration, resolution, config)
	if err != nil {
		return "", fmt.Errorf("failed to create video: %w", err)
	}

	// Get file size
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to get video file info: %w", err)
	}

	// Create response
	result := map[string]interface{}{
		"file_id":   fmt.Sprintf("%d", time.Now().UnixNano()), // In Go we don't have file IDs like Drupal
		"uri":       outputPath,
		"url":       fmt.Sprintf("/storage/pipeline/videos/%s/%s", time.Now().Format("2006-01"), filename),
		"mime_type": fmt.Sprintf("video/%s", outputFormat),
		"filename":  filename,
		"duration":  duration,
		"size":      fileInfo.Size(),
		"timestamp": time.Now().Unix(),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("error marshaling result: %w", err)
	}

	s.logger.Info("Video generation completed successfully",
		slog.String("output_path", outputPath),
		slog.Float64("duration", duration))

	return string(resultJSON), nil
}

func (s *VideoGenerationActionService) findFileInfo(pipelineContext *pipeline_type.Context, outputType string) (*FileInfo, error) {
	s.logger.Debug("Looking for file info",
		slog.String("output_type", outputType),
		slog.Any("available_keys", getMapKeys(pipelineContext.StepOutputs)))

	// For featured_image, first try the direct "image_data" key
	if outputType == "featured_image" && pipelineContext.StepOutputs["image_data"] != nil {
		imageData := pipelineContext.StepOutputs["image_data"]
		
		// Handle image URL directly (DALL-E response)
		if imageURL, ok := imageData.(string); ok {
			s.logger.Debug("Found image URL", slog.String("url", imageURL))
			
			// Download the image to a local file
			imageFilePath, err := s.downloadImage(imageURL)
			if err != nil {
				return nil, fmt.Errorf("failed to download image: %w", err)
			}
			
			// Create a file info structure for the downloaded image
			fileInfo := &FileInfo{
				FileID:    fmt.Sprintf("%d", time.Now().UnixNano()),
				URI:       imageFilePath,
				URL:       imageURL,
				MimeType:  "image/jpeg", // Assume JPEG, we could try to detect from URL or content
				Filename:  filepath.Base(imageFilePath),
				Size:      0, // We could get the actual size if needed
				Timestamp: time.Now().Unix(),
			}
			
			return fileInfo, nil
		}
	}
	
	// For audio_content, first try the direct "audio_data" key
	if outputType == "audio_content" && pipelineContext.StepOutputs["audio_data"] != nil {
		audioData := pipelineContext.StepOutputs["audio_data"]
		
		// Handle audio data as JSON string
		if audioJSON, ok := audioData.(string); ok && s.isJson(audioJSON) {
			s.logger.Debug("Found audio JSON data")
			
			var fileInfo FileInfo
			if err := json.Unmarshal([]byte(audioJSON), &fileInfo); err == nil {
				s.logger.Debug("Successfully parsed audio file info",
					slog.String("uri", fileInfo.URI))
				return &fileInfo, nil
			} else {
				s.logger.Error("Failed to parse audio JSON",
					slog.String("error", err.Error()),
					slog.String("data", audioJSON))
			}
		}
	}

	// General fallback - search all steps for matching output type
	for key, value := range pipelineContext.StepOutputs {
		// If the value is a JSON string
		if strValue, ok := value.(string); ok && s.isJson(strValue) {
			var fileInfo FileInfo
			if err := json.Unmarshal([]byte(strValue), &fileInfo); err == nil {
				// Check if this matches our target output type based on MIME type
				if (outputType == "featured_image" && strings.Contains(fileInfo.MimeType, "image")) ||
				   (outputType == "audio_content" && strings.Contains(fileInfo.MimeType, "audio")) {
					s.logger.Debug("Found file info from JSON by MIME type",
						slog.String("key", key),
						slog.String("mime_type", fileInfo.MimeType))
					return &fileInfo, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no file info found with output type: %s", outputType)
}

// Helper function to download an image from a URL to a local file
func (s *VideoGenerationActionService) downloadImage(imageURL string) (string, error) {
	// Create a directory for downloaded images
	dir := filepath.Join("storage", "pipeline", "images", time.Now().Format("2006-01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Generate filename for the downloaded image
	filename := fmt.Sprintf("image_%d.jpg", time.Now().UnixNano())
	outputPath := filepath.Join(dir, filename)
	
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	// Download the image
	s.logger.Debug("Downloading image", slog.String("url", imageURL), slog.String("to", outputPath))
	
	resp, err := client.Get(imageURL)
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
	
	s.logger.Info("Successfully downloaded image", slog.String("path", outputPath))
	return outputPath, nil
}

// Helper function to get map keys for logging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function to check if a string is valid JSON
func (s *VideoGenerationActionService) isJson(str string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}


func (s *VideoGenerationActionService) uriToFilePath(uri string) string {
	// Handle different URI formats based on your system
	// This is a simplified example, adjust based on your actual URI format
	if strings.HasPrefix(uri, "/") {
		// Already an absolute path
		return uri
	}
	
	// Remove scheme if present (like file://)
	if strings.Contains(uri, "://") {
		uri = uri[strings.Index(uri, "://")+3:]
	}
	
	return uri
}


func (s *VideoGenerationActionService) getAudioDuration(filePath string) (float64, error) {
	cmd := exec.Command("ffprobe", "-i", filePath, "-show_entries", "format=duration", "-v", "quiet", "-of", "csv=p=0")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe execution failed: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

func (s *VideoGenerationActionService) createVideo(imagePath, audioPath, outputPath string, duration float64, resolution string, config map[string]interface{}) error {
	// Create FFmpeg command
	args := []string{
		"-loop", "1",
		"-i", imagePath,
		"-i", audioPath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-t", fmt.Sprintf("%.6f", duration),
		"-pix_fmt", "yuv420p",
		"-shortest",
	}

	// Add resolution if specified
	if resolution != "" {
		args = append(args, "-s", resolution)
	}

	// Add bitrate if specified
	if bitrate, ok := config["bitrate"].(string); ok && bitrate != "" {
		args = append(args, "-b:v", bitrate)
	}

	// Add framerate if specified
	if framerate, ok := config["framerate"].(float64); ok && framerate > 0 {
		args = append(args, "-r", fmt.Sprintf("%.0f", framerate))
	}

	// Add output file
	args = append(args, "-y", outputPath)

	// Log the command
	s.logger.Debug("Executing FFmpeg command", slog.Any("args", args))

	// Execute command
	cmd := exec.Command("ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	// Read stderr for debugging
	stderrOutput, _ := io.ReadAll(stderr)

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		s.logger.Error("FFmpeg execution failed",
			slog.String("error", err.Error()),
			slog.String("stderr", string(stderrOutput)))
		return fmt.Errorf("FFmpeg execution failed: %w", err)
	}

	// Verify output file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return fmt.Errorf("FFmpeg did not create an output file")
	}

	return nil
}

func (s *VideoGenerationActionService) getResolution(quality string) string {
	switch quality {
	case "low":
		return "640x480"
	case "high":
		return "1920x1080"
	case "medium":
		fallthrough
	default:
		return "1280x720"
	}
}

func (s *VideoGenerationActionService) CanHandle(actionService string) bool {
	return actionService == VideoGenerationServiceName
}