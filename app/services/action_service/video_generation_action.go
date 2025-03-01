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

// FileInfo represents standardized file information
type FileInfo struct {
	FileID    int64  `json:"file_id"`
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
		
	// Find image and audio files from the pipeline context based on output types
	imageFileInfo, err := s.findFileByOutputType(pipelineContext, "featured_image")
	if err != nil {
		return "", fmt.Errorf("image file information not found: %w", err)
	}

	audioFileInfo, err := s.findFileByOutputType(pipelineContext, "audio_content")
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
		"file_id":   fmt.Sprintf("%d", time.Now().UnixNano()),
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

// findFileByOutputType looks for files matching a specific output type in the pipeline context
func (s *VideoGenerationActionService) findFileByOutputType(pipelineContext *pipeline_type.Context, outputType string) (*FileInfo, error) {
	s.logger.Info("Looking for file with output type", slog.String("output_type", outputType))
	
	// First approach: Get all steps with the matching output type and check their outputs
	steps := pipelineContext.GetStepsByOutputType(outputType)
	s.logger.Debug("Found steps with matching output type", 
		slog.Int("count", len(steps)),
		slog.Any("step_ids", getStepIDs(steps)))
	
	for _, step := range steps {
		if stepOutput, exists := pipelineContext.GetStepOutput(step.StepOutputKey); exists {
			// Try to handle different formats of output
			fileInfo, err := s.parseFileInfo(stepOutput, outputType)
			if err == nil {
				s.logger.Info("Found file info from step with matching output_type",
					slog.String("step_id", step.ID),
					slog.String("output_key", step.StepOutputKey),
					slog.String("uri", fileInfo.URI))
				return fileInfo, nil
			} else {
				s.logger.Debug("Could not parse file info from step", 
					slog.String("step_id", step.ID), 
					slog.String("error", err.Error()))
			}
		}
	}
	
	// Second approach: Special case for common keys
	if outputType == "featured_image" && pipelineContext.StepOutputs["image_data"] != nil {
		fileInfo, err := s.parseFileInfo(pipelineContext.StepOutputs["image_data"], outputType)
		if err == nil {
			return fileInfo, nil
		}
	}
	
	if outputType == "audio_content" && pipelineContext.StepOutputs["audio_data"] != nil {
		fileInfo, err := s.parseFileInfo(pipelineContext.StepOutputs["audio_data"], outputType)
		if err == nil {
			return fileInfo, nil
		}
	}
	
	// Final approach: Scan all outputs and check if they match the expected type
	for key, value := range pipelineContext.StepOutputs {
		s.logger.Debug("Trying step output", slog.String("key", key))
		
		fileInfo, err := s.parseFileInfo(value, outputType)
		if err == nil {
			s.logger.Info("Found file info from step output scanning",
				slog.String("key", key),
				slog.String("uri", fileInfo.URI))
			return fileInfo, nil
		}
	}
	
	return nil, fmt.Errorf("no file info found with output type: %s", outputType)
}

// parseFileInfo attempts to extract file information from different formats of step outputs
func (s *VideoGenerationActionService) parseFileInfo(output interface{}, outputType string) (*FileInfo, error) {
	// Case 1: Direct URL (e.g., OpenAI image output)
	if url, ok := output.(string); ok {
		// Check if it's a URL that matches the expected type
		if outputType == "featured_image" && isImageURL(url) {
			s.logger.Debug("Got direct image URL", slog.String("url", url))
			
			// Download the image to a local file
			localFilePath, err := s.downloadFile(url, "images")
			if err != nil {
				return nil, fmt.Errorf("failed to download image: %w", err)
			}
			
			// Create a file info structure for the downloaded image
			fileInfo := &FileInfo{
				FileID:    0,
				URI:       localFilePath,
				URL:       url,
				MimeType:  detectMimeType(url, "image/jpeg"),
				Filename:  filepath.Base(localFilePath),
				Size:      0,
				Timestamp: time.Now().Unix(),
			}
			
			return fileInfo, nil
		}
		
		// Check if it's a JSON string
		if isJSON(url) {
			// First try with a custom struct that matches the actual format from the service
			var audioResponse struct {
				FileID    string `json:"file_id"`
				URI       string `json:"uri"`
				URL       string `json:"url"`
				MimeType  string `json:"mime_type"`
				Filename  string `json:"filename"`
				Size      int64  `json:"size"`
				Timestamp int64  `json:"timestamp"`
			}
			
			if err := json.Unmarshal([]byte(url), &audioResponse); err == nil && audioResponse.URI != "" {
				// Convert to our standard FileInfo format
				var fileID int64 = 0
				if id, err := strconv.ParseInt(audioResponse.FileID, 10, 64); err == nil {
					fileID = id
				}
				
				fileInfo := &FileInfo{
					FileID:    fileID,
					URI:       audioResponse.URI,
					URL:       audioResponse.URL,
					MimeType:  audioResponse.MimeType,
					Filename:  audioResponse.Filename,
					Size:      audioResponse.Size,
					Timestamp: audioResponse.Timestamp,
				}
				
				// Check if this matches the expected output type
				if outputType == "featured_image" && !isImageType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "images") {
					return nil, fmt.Errorf("file info doesn't match expected image type")
				}
				
				if outputType == "audio_content" && !isAudioType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "audio") {
					return nil, fmt.Errorf("file info doesn't match expected audio type")
				}
				
				return fileInfo, nil
			}
			
			// If that fails, try with the standard FileInfo struct as a fallback
			var fileInfo FileInfo
			if err := json.Unmarshal([]byte(url), &fileInfo); err != nil {
				return nil, fmt.Errorf("not valid file info JSON: %w", err)
			}
			
			// Validate the file info matches the expected output type
			if outputType == "featured_image" && !isImageType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "images") {
				return nil, fmt.Errorf("file info doesn't match expected image type")
			}
			
			if outputType == "audio_content" && !isAudioType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "audio") {
				return nil, fmt.Errorf("file info doesn't match expected audio type")
			}
			
			return &fileInfo, nil
		}
		
		return nil, fmt.Errorf("output is a string but not a valid file info format")
	}
	
	// Case 2: Map interface (could be a file info object)
	if mapData, ok := output.(map[string]interface{}); ok {
		// Check if it has the expected fields for a file info
		if uri, ok := mapData["uri"].(string); ok && uri != "" {
			fileInfo := FileInfo{
				URI: uri,
			}
			
			if url, ok := mapData["url"].(string); ok {
				fileInfo.URL = url
			}
			
			if mimeType, ok := mapData["mime_type"].(string); ok {
				fileInfo.MimeType = mimeType
			}
			
			if filename, ok := mapData["filename"].(string); ok {
				fileInfo.Filename = filename
			}
			
			// Try to extract FileID
			if fileID, ok := mapData["file_id"]; ok {
				switch v := fileID.(type) {
				case int:
					fileInfo.FileID = int64(v)
				case int64:
					fileInfo.FileID = v
				case float64:
					fileInfo.FileID = int64(v)
				case string:
					if id, err := strconv.ParseInt(v, 10, 64); err == nil {
						fileInfo.FileID = id
					}
				}
			}
			
			// Try to extract Size
			if size, ok := mapData["size"]; ok {
				switch v := size.(type) {
				case int:
					fileInfo.Size = int64(v)
				case int64:
					fileInfo.Size = v
				case float64:
					fileInfo.Size = int64(v)
				}
			}
			
			// Validate the file info matches the expected output type
			if outputType == "featured_image" && !isImageType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "images") {
				return nil, fmt.Errorf("file info doesn't match expected image type")
			}
			
			if outputType == "audio_content" && !isAudioType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "audio") {
				return nil, fmt.Errorf("file info doesn't match expected audio type")
			}
			
			return &fileInfo, nil
		}
	}
	
	return nil, fmt.Errorf("output is not in a recognized file info format")
}

// downloadFile downloads a file from a URL to a local directory
func (s *VideoGenerationActionService) downloadFile(fileURL string, fileType string) (string, error) {
	// Create a directory for downloaded files
	dir := filepath.Join("storage", "pipeline", fileType, time.Now().Format("2006-01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Generate filename
	extension := "dat"
	if fileType == "images" {
		extension = "jpg"
	} else if fileType == "audio" {
		extension = "mp3"
	}
	
	filename := fmt.Sprintf("%s_%d.%s", fileType[:3], time.Now().UnixNano(), extension)
	outputPath := filepath.Join(dir, filename)
	
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	
	// Download the file
	s.logger.Debug("Downloading file", slog.String("url", fileURL), slog.String("to", outputPath))
	
	resp, err := client.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file, status: %d", resp.StatusCode)
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
		return "", fmt.Errorf("failed to save file data: %w", err)
	}
	
	s.logger.Info("Successfully downloaded file", slog.String("path", outputPath))
	return outputPath, nil
}

func (s *VideoGenerationActionService) uriToFilePath(uri string) string {
	// Handle different URI formats
	if strings.HasPrefix(uri, "/") {
		// Already an absolute path
		return uri
	}
	
	// For file paths from upload steps, they should already be local paths
	if strings.HasPrefix(uri, "storage/") {
		// Return as is - it's a local path
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

// Helper functions

func isJSON(str string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}

func isImageURL(url string) bool {
	// Check if it's a URL that seems to point to an image
	return strings.HasPrefix(url, "http") && 
		(strings.Contains(url, ".jpg") || 
		 strings.Contains(url, ".jpeg") || 
		 strings.Contains(url, ".png") || 
		 strings.Contains(url, ".webp") || 
		 strings.Contains(url, ".gif") || 
		 strings.Contains(url, "image"))
}

func isImageType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

func isAudioType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "audio/")
}

func detectMimeType(url string, defaultMime string) string {
	if strings.Contains(url, ".jpg") || strings.Contains(url, ".jpeg") {
		return "image/jpeg"
	}
	if strings.Contains(url, ".png") {
		return "image/png"
	}
	if strings.Contains(url, ".webp") {
		return "image/webp"
	}
	if strings.Contains(url, ".gif") {
		return "image/gif"
	}
	if strings.Contains(url, ".mp3") {
		return "audio/mpeg"
	}
	if strings.Contains(url, ".wav") {
		return "audio/wav"
	}
	return defaultMime
}

func getStepIDs(steps []pipeline_type.PipelineStep) []string {
	ids := make([]string, len(steps))
	for i, step := range steps {
		ids[i] = step.ID
	}
	return ids
}