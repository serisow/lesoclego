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
	FileID    int64                  `json:"file_id"`
	URI       string                 `json:"uri"`
	URL       string                 `json:"url"`
	MimeType  string                 `json:"mime_type"`
	Filename  string                 `json:"filename"`
	Size      int64                  `json:"size"`
	Timestamp int64                  `json:"timestamp"`
	Duration  float64                `json:"duration,omitempty"`
	StepKey   string                 `json:"step_key,omitempty"`
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
		
	// Find all images with output type "featured_image"
	imageFiles, err := s.findFilesByOutputType(pipelineContext, "featured_image")
	if err != nil {
		return "", fmt.Errorf("image files not found: %w", err)
	}

	// Find audio file with output type "audio_content"
	audioFileInfo, err := s.findFileByOutputType(pipelineContext, "audio_content")
	if err != nil {
		return "", fmt.Errorf("audio file information not found: %w", err)
	}

	s.logger.Debug("Found required files",
		slog.Int("image_count", len(imageFiles)),
		slog.String("audio_uri", audioFileInfo.URI))

	// Get file paths from URIs
	imagePaths := make([]string, len(imageFiles))
	for i, imageFile := range imageFiles {
		imagePaths[i] = s.uriToFilePath(imageFile.URI)
		
		// Verify file exists
		if _, err := os.Stat(imagePaths[i]); os.IsNotExist(err) {
			return "", fmt.Errorf("image file not found at path: %s", imagePaths[i])
		}
	}
	
	audioFilePath := s.uriToFilePath(audioFileInfo.URI)
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
	audioDuration, err := s.getAudioDuration(audioFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Calculate durations for each image
	imageDurations := s.calculateImageDurations(imageFiles, audioDuration)

	// Create video using FFmpeg
	err = s.createMultiImageVideo(imagePaths, imageDurations, audioFilePath, outputPath, resolution, config)
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
		slides[i] = map[string]interface{}{
			"file_id":  imageFile.FileID,
			"duration": imageDurations[i],
			"step_key": imageFile.StepKey,
		}
	}

	// Create response
	result := map[string]interface{}{
		"file_id":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"uri":       outputPath,
		"url":       fmt.Sprintf("/storage/pipeline/videos/%s/%s", time.Now().Format("2006-01"), filename),
		"mime_type": fmt.Sprintf("video/%s", outputFormat),
		"filename":  filename,
		"duration":  audioDuration,
		"size":      fileInfo.Size(),
		"timestamp": time.Now().Unix(),
		"slides":    slides,
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

// findFilesByOutputType returns all files matching a specific output type
func (s *VideoGenerationActionService) findFilesByOutputType(pipelineContext *pipeline_type.Context, outputType string) ([]*FileInfo, error) {
	s.logger.Info("Looking for files with output type", slog.String("output_type", outputType))
	
	var files []*FileInfo
	
	// Find all steps with matching output_type
	steps := pipelineContext.GetStepsByOutputType(outputType)
	s.logger.Debug("Found steps with matching output type", 
		slog.Int("count", len(steps)),
		slog.Any("step_ids", getStepIDs(steps)))
	
	// First, try to find by exact output_type match
	for _, step := range steps {
		if stepOutput, exists := pipelineContext.GetStepOutput(step.StepOutputKey); exists {
			fileInfo, err := s.parseFileInfo(stepOutput, outputType)
			if err == nil {
				fileInfo.StepKey = step.StepOutputKey
				files = append(files, fileInfo)
				s.logger.Info("Found file info from step with matching output_type",
					slog.String("step_id", step.ID),
					slog.String("output_key", step.StepOutputKey))
			} else {
				s.logger.Debug("Could not parse file info from step", 
					slog.String("step_id", step.ID), 
					slog.String("error", err.Error()))
			}
		}
	}
	
	// If no files found through direct output_type match, try known keys
	if len(files) == 0 && outputType == "featured_image" {
		// Look for keys containing "image_data"
		for key, value := range pipelineContext.StepOutputs {
			if strings.Contains(key, "image_data") {
				fileInfo, err := s.parseFileInfo(value, outputType)
				if err == nil {
					fileInfo.StepKey = key
					files = append(files, fileInfo)
					s.logger.Info("Found file info from key scan",
						slog.String("key", key),
						slog.String("uri", fileInfo.URI))
				}
			}
		}
	}
	
	// Sort files by their step weights if available
	if len(files) > 1 {
		s.sortFilesByStepWeight(files, pipelineContext)
	}
	
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found with output type: %s", outputType)
	}
	
	return files, nil
}

// sortFilesByStepWeight sorts the files based on their step weights
func (s *VideoGenerationActionService) sortFilesByStepWeight(files []*FileInfo, pipelineContext *pipeline_type.Context) {
	// Create a map of step output key to weight
	weightMap := make(map[string]int)
	for _, step := range pipelineContext.Steps {
		weightMap[step.StepOutputKey] = step.Weight
	}
	
	// Sort files based on their step weights
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			iWeight := weightMap[files[i].StepKey]
			jWeight := weightMap[files[j].StepKey]
			
			if iWeight > jWeight {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}

// findFileByOutputType looks for a single file matching a specific output type
func (s *VideoGenerationActionService) findFileByOutputType(pipelineContext *pipeline_type.Context, outputType string) (*FileInfo, error) {
	s.logger.Info("Looking for file with output type", slog.String("output_type", outputType))
	
	// First approach: Look for steps that have the specific output_type
	steps := pipelineContext.GetStepsByOutputType(outputType)
	s.logger.Debug("Found steps with matching output type", 
		slog.Int("count", len(steps)),
		slog.Any("step_ids", getStepIDs(steps)))
	
	for _, step := range steps {
		if stepOutput, exists := pipelineContext.GetStepOutput(step.StepOutputKey); exists {
			fileInfo, err := s.parseFileInfo(stepOutput, outputType)
			if err == nil {
				s.logger.Info("Found file info from step with matching output_type",
					slog.String("step_id", step.ID),
					slog.String("output_key", step.StepOutputKey),
					slog.String("uri", fileInfo.URI))
				return fileInfo, nil
			}
		}
	}
	
	// Second approach: Try common step output keys
	if outputType == "audio_content" && pipelineContext.StepOutputs["audio_data"] != nil {
		fileInfo, err := s.parseFileInfo(pipelineContext.StepOutputs["audio_data"], outputType)
		if err == nil {
			return fileInfo, nil
		}
	}
	
	// Final approach: Scan all outputs as a last resort
	for key, value := range pipelineContext.StepOutputs {
		if strings.Contains(key, "audio") {
			fileInfo, err := s.parseFileInfo(value, outputType)
			if err == nil {
				s.logger.Info("Found file info from step output scanning",
					slog.String("key", key),
					slog.String("uri", fileInfo.URI))
				return fileInfo, nil
			}
		}
	}
	
	return nil, fmt.Errorf("no file info found with output type: %s", outputType)
}

// calculateImageDurations distributes the audio duration across images
func (s *VideoGenerationActionService) calculateImageDurations(imageFiles []*FileInfo, audioDuration float64) []float64 {
	imageCount := len(imageFiles)
	if imageCount == 0 {
		return []float64{}
	}
	
	// Distribute equally by default
	equalDuration := audioDuration / float64(imageCount)
	imageDurations := make([]float64, imageCount)
	for i := range imageDurations {
		imageDurations[i] = equalDuration
	}
	
	// Log the calculated durations
	for i, duration := range imageDurations {
		s.logger.Debug("Image duration set",
			slog.Int("index", i),
			slog.Float64("duration", duration),
			slog.String("file", imageFiles[i].Filename))
	}
	
	return imageDurations
}

// Handle structured file information in JSON format
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
			// First try with a custom struct that matches the actual format from services
			// Many audio services return file_id as a string rather than a number
			var audioResponse struct {
				FileID    string  `json:"file_id"`
				URI       string  `json:"uri"`
				URL       string  `json:"url"`
				MimeType  string  `json:"mime_type"`
				Filename  string  `json:"filename"`
				Size      int64   `json:"size"`
				Duration  float64 `json:"duration,omitempty"`
				Timestamp int64   `json:"timestamp"`
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
					Duration:  audioResponse.Duration,
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
			
			// Try to extract Duration if available
			if duration, ok := mapData["duration"].(float64); ok {
				fileInfo.Duration = duration
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

// uriToFilePath converts a URI to a file path
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

// getAudioDuration gets the duration of an audio file using ffprobe
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

// createMultiImageVideo creates a video from multiple images and an audio file
func (s *VideoGenerationActionService) createMultiImageVideo(imagePaths []string, imageDurations []float64, audioPath string, outputPath string, resolution string, config map[string]interface{}) error {
	if len(imagePaths) == 0 {
		return fmt.Errorf("no image paths provided")
	}
	
	// Prepare filter complex for concatenating images
	filterComplex := ""
	segments := []string{}
	
	// Process each image with its duration
	for i, _ := range imagePaths {
		// Get duration for this image
		duration := imageDurations[i]
		
		// Add scaling and setsar for proper aspect ratio
		filterComplex += fmt.Sprintf("[%d:v]loop=1:size=1,trim=duration=%.3f,setpts=PTS-STARTPTS,scale=%s:force_divisible_by=2,setsar=1[v%d];", 
			i, duration, resolution, i)
		segments = append(segments, fmt.Sprintf("[v%d]", i))
	}
	
	// Concatenate all segments
	filterComplex += strings.Join(segments, "") + fmt.Sprintf("concat=n=%d:v=1:a=0[outv]", len(imagePaths))
	
	// Build ffmpeg command
	args := []string{}
	
	// Add inputs for each image
	for _, imagePath := range imagePaths {
		args = append(args, "-loop", "1", "-i", imagePath)
	}
	
	// Add audio input
	args = append(args, "-i", audioPath)
	
	// Add filter complex
	args = append(args, "-filter_complex", filterComplex)
	
	// Add mapping
	args = append(args, "-map", "[outv]", "-map", fmt.Sprintf("%d:a", len(imagePaths)))
	
	// Add encoding parameters
	args = append(args, 
		"-c:v", "libx264",
		"-c:a", "aac",
		"-pix_fmt", "yuv420p")
	
	// Add bitrate if specified
	if bitrate, ok := config["bitrate"].(string); ok && bitrate != "" {
		args = append(args, "-b:v", bitrate)
	}
	
	// Add framerate if specified
	if framerate, ok := config["framerate"].(float64); ok && framerate > 0 {
		args = append(args, "-r", fmt.Sprintf("%.0f", framerate))
	}
	
	// Add shortest flag to make output duration match audio
	args = append(args, "-shortest")
	
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

// getResolution returns the resolution based on the quality setting
func (s *VideoGenerationActionService) getResolution(quality string) string {
	switch quality {
	case "low":
		return "640:480"
	case "high":
		return "1920:1080"
	case "medium":
		fallthrough
	default:
		return "1280:720"
	}
}

// Helper function to get step IDs for logging
func getStepIDs(steps []pipeline_type.PipelineStep) []string {
	ids := make([]string, len(steps))
	for i, step := range steps {
		ids[i] = step.ID
	}
	return ids
}

// Helper functions for file type detection

// isJSON checks if a string is valid JSON
func isJSON(str string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}

// isImageURL checks if a URL appears to point to an image
func isImageURL(url string) bool {
	return strings.HasPrefix(url, "http") && 
		(strings.Contains(url, ".jpg") || 
		 strings.Contains(url, ".jpeg") || 
		 strings.Contains(url, ".png") || 
		 strings.Contains(url, ".webp") || 
		 strings.Contains(url, ".gif") || 
		 strings.Contains(url, "image"))
}

// isImageType checks if a MIME type is an image
func isImageType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

// isAudioType checks if a MIME type is audio
func isAudioType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "audio/")
}

// detectMimeType guesses MIME type from a URL
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

func (s *VideoGenerationActionService) CanHandle(actionService string) bool {
	return actionService == VideoGenerationServiceName
}