package video

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"

)

// FFmpegExecutorImpl implements the FFmpegExecutor interface
type FFmpegExecutorImpl struct {
	logger       *slog.Logger
	textProcessor TextProcessor
}

// NewFFmpegExecutor creates a new FFmpeg executor instance
func NewFFmpegExecutor(logger *slog.Logger, textProcessor TextProcessor) FFmpegExecutor {
	return &FFmpegExecutorImpl{
		logger:       logger,
		textProcessor: textProcessor,
	}
}

// GetAudioDuration gets the duration of an audio file using ffprobe
func (fe *FFmpegExecutorImpl) GetAudioDuration(filePath string) (float64, error) {
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

// GetResolution returns the resolution based on the quality setting
func (fe *FFmpegExecutorImpl) GetResolution(quality string) string {
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


// CalculateImageDurations calculates the duration for each image to match audio duration
func (fe *FFmpegExecutorImpl) CalculateImageDurations(sourceData interface{}, audioDuration float64, transitionDuration float64) []float64 {
	var imageDurations []float64
	var imageCount int

	// Handle different input types
	switch source := sourceData.(type) {
	case []*FileInfo:
		// Case 1: Called with image files (from Execute)
		imageCount = len(source)
		imageDurations = make([]float64, imageCount)

		// Extract durations from file info
		for i, file := range source {
			if file.Duration > 0 {
				imageDurations[i] = file.Duration
			} else {
				// Default duration if not specified
				imageDurations[i] = audioDuration / float64(imageCount)
			}
		}

	case []float64:
		// Case 2: Called with existing durations (from createMultiImageVideo)
		imageDurations = make([]float64, len(source))
		copy(imageDurations, source)
		imageCount = len(source)

	default:
		// Unexpected input type
		fe.logger.Error("Invalid source data type for calculateImageDurations",
			slog.String("type", fmt.Sprintf("%T", sourceData)))
		return []float64{}
	}

	if imageCount == 0 {
		return []float64{}
	}

	// Calculate total duration
	totalDuration := 0.0
	for _, duration := range imageDurations {
		totalDuration += duration
	}

	// Calculate total transition time (transitions overlap with image durations)
	totalTransitionTime := transitionDuration * float64(imageCount-1)

	// Log timing details for debugging
	fe.logger.Debug("Timing details",
		slog.Float64("audio_duration", audioDuration),
		slog.Float64("total_image_duration", totalDuration),
		slog.Float64("transition_time", totalTransitionTime))

	// Adjust image durations to precisely match audio duration
	if audioDuration > 0 && math.Abs(totalDuration-totalTransitionTime-audioDuration) > 0.1 {
		scaleFactor := audioDuration / (totalDuration - totalTransitionTime)
		for i := range imageDurations {
			imageDurations[i] *= scaleFactor
		}

		// Recalculate total duration after adjustment
		var adjustedTotal float64
		for _, d := range imageDurations {
			adjustedTotal += d
		}

		fe.logger.Debug("Adjusted durations",
			slog.Any("durations", imageDurations),
			slog.Float64("total_after_adjustment", adjustedTotal))
	}

	// Log the calculated durations
	for i, duration := range imageDurations {
		fe.logger.Debug("Image duration set",
			slog.Int("index", i),
			slog.Float64("duration", duration))
	}

	return imageDurations
}

// CreateMultiImageVideo creates a video from multiple images and an audio file
func (fe *FFmpegExecutorImpl) CreateMultiImageVideo(params VideoParams) error {
	imagePaths := make([]string, len(params.ImageFiles))
	for i, imageFile := range params.ImageFiles {
		imagePaths[i] = imageFile.URI
	}

	if len(imagePaths) == 0 {
		return fmt.Errorf("no image paths provided")
	}

	// Get transition configuration
	transitionType := getStringValue(params.Config, "transition_type", "fade")
	transitionDurationStr := getStringValue(params.Config, "transition_duration", "1")
	transitionDuration, err := strconv.ParseFloat(transitionDurationStr, 64)
	if err != nil {
		// Default to 1 second if parsing fails
		transitionDuration = 1.0
		fe.logger.Warn("Failed to parse transition_duration, using default",
			slog.String("value", transitionDurationStr),
			slog.Float64("default", transitionDuration))
	}

	// Build ffmpeg command
	args := []string{}

	// Add inputs for each image
	for _, imagePath := range imagePaths {
		args = append(args, "-loop", "1", "-i", imagePath)
	}

	// Add audio input
	args = append(args, "-i", params.AudioFile.URI)

	// Create filter complex string for transitions with text overlays
	filterComplex := ""

	// Scale and format each image, adding text overlay if configured
	for i := 0; i < len(imagePaths); i++ {
		// Basic image scaling filter
		imgFilter := fmt.Sprintf("[%d:v]scale=%s:force_divisible_by=2,setsar=1,format=yuv420p",
			i, params.Resolution)

		// Complete this image's filter chain
		imgFilter += fmt.Sprintf("[v%d]", i)

		// Add to the overall filter complex
		if filterComplex != "" {
			filterComplex += ";"
		}
		filterComplex += imgFilter
	}

	// Add trimming for each image
	for i := 0; i < len(imagePaths); i++ {
		if filterComplex != "" {
			filterComplex += ";"
		}
		filterComplex += fmt.Sprintf("[v%d]trim=duration=%.3f,setpts=PTS-STARTPTS[hold%d]",
			i, params.ImageDurations[i], i)
	}

	// If there's only one image, just use it directly
	if len(imagePaths) == 1 {
		args = append(args, "-filter_complex", filterComplex)
		args = append(args, "-map", "[hold0]", "-map", fmt.Sprintf("%d:a", len(imagePaths)))
	} else {
		// Generate transitions between images
		currentOffset := params.ImageDurations[0] - transitionDuration
		lastOutput := "hold0"

		for i := 1; i < len(imagePaths); i++ {
			// Ensure offset is not negative
			if currentOffset < 0 {
				currentOffset = 0
			}

			// Add transition to filter complex
			if filterComplex != "" {
				filterComplex += ";"
			}
			filterComplex += fmt.Sprintf("[%s][hold%d]xfade=transition=%s:duration=%.3f:offset=%.3f[trans%d]",
				lastOutput, i, transitionType, transitionDuration, currentOffset, i)

			lastOutput = fmt.Sprintf("trans%d", i)

			// Update offset for next transition
			currentOffset += params.ImageDurations[i] - transitionDuration
		}

		// Add filter complex to command
		args = append(args, "-filter_complex", filterComplex)

		// Map final output and audio
		args = append(args, "-map", fmt.Sprintf("[%s]", lastOutput), "-map", fmt.Sprintf("%d:a", len(imagePaths)))
	}

	// Add encoding parameters
	args = append(args,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-pix_fmt", "yuv420p")

	// Add bitrate if specified
	if bitrate, ok := params.Config["bitrate"].(string); ok && bitrate != "" {
		args = append(args, "-b:v", bitrate)
	}

	// Add framerate if specified
	frameRateStr, ok := params.Config["framerate"].(string)
	if ok && frameRateStr != "" {
		frameRate, err := strconv.ParseFloat(frameRateStr, 64)
		if err == nil && frameRate > 0 {
			args = append(args, "-r", fmt.Sprintf("%.0f", frameRate))
		}
	} else if frameRate, ok := params.Config["framerate"].(float64); ok && frameRate > 0 {
		args = append(args, "-r", fmt.Sprintf("%.0f", frameRate))
	}

	// Add shortest flag to make output duration match audio
	args = append(args, "-shortest")

	// Add output file
	args = append(args, "-y", params.OutputPath)

	// Log the command for debugging
	fe.logger.Debug("Executing FFmpeg command", slog.Any("args", args))

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
		fe.logger.Error("FFmpeg execution failed",
			slog.String("error", err.Error()),
			slog.String("stderr", string(stderrOutput)))
		return fmt.Errorf("FFmpeg execution failed: %w", err)
	}

	// Verify output file exists
	if _, err := os.Stat(params.OutputPath); os.IsNotExist(err) {
		return fmt.Errorf("FFmpeg did not create an output file")
	}

	return nil
}