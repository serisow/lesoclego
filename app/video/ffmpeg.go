package video

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// FFmpegExecutorImpl implements the FFmpegExecutor interface
type FFmpegExecutorImpl struct {
	logger        *slog.Logger
	textProcessor TextProcessor
}

// NewFFmpegExecutor creates a new FFmpeg executor instance
func NewFFmpegExecutor(logger *slog.Logger, textProcessor TextProcessor) FFmpegExecutor {
	return &FFmpegExecutorImpl{
		logger:        logger,
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
// Add Ken Burns and enhanced text animation support
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

	// Parse resolution to get width and height
	parts := strings.Split(params.Resolution, ":")
	if len(parts) == 2 {
		if w, err := strconv.Atoi(parts[0]); err == nil {
			params.Width = w
		}
		if h, err := strconv.Atoi(parts[1]); err == nil {
			params.Height = h
		}
	}

	// Check Ken Burns configuration
	kenBurnsEnabled := false
	kenBurnsStyle := "random"
	kenBurnsIntensity := "moderate"

	if kenBurnsConfig, ok := params.Config["ken_burns"].(map[string]interface{}); ok {
		// Check enabled flag based on various possible types
		if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(float64); ok {
			kenBurnsEnabled = enabled == 1
		} else if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(bool); ok {
			kenBurnsEnabled = enabled
		} else if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(string); ok {
			kenBurnsEnabled = enabled == "1" || strings.ToLower(enabled) == "true"
		} else if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(int); ok {
			kenBurnsEnabled = enabled == 1
		}

		// Get style and intensity if provided
		if style, ok := kenBurnsConfig["ken_burns_style"].(string); ok && style != "" {
			kenBurnsStyle = style
		}
		if intensity, ok := kenBurnsConfig["ken_burns_intensity"].(string); ok && intensity != "" {
			kenBurnsIntensity = intensity
		}
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

	// Process each image - scale and add text blocks
	for i := 0; i < len(imagePaths); i++ {
		// Basic image scaling filter
		imgFilter := fmt.Sprintf("[%d:v]scale=%s:force_divisible_by=2,setsar=1,format=yuv420p",
			i, params.Resolution)

		// Add Ken Burns effect if enabled
		if kenBurnsEnabled {
			// Get framerate
			framerate := 24 // Default
			if frameRateStr, ok := params.Config["framerate"].(string); ok {
				if fr, err := strconv.Atoi(frameRateStr); err == nil && fr > 0 {
					framerate = fr
				}
			} else if frameRate, ok := params.Config["framerate"].(float64); ok && frameRate > 0 {
				framerate = int(frameRate)
			}

			kenBurnsFilter := fe.getKenBurnsParameters(
				kenBurnsStyle,
				kenBurnsIntensity,
				i,
				params.ImageDurations[i],
				framerate)

			imgFilter += "," + kenBurnsFilter
		}

		// Add text blocks if available for this image
		if i < len(params.ImageFiles) && len(params.ImageFiles[i].TextBlocks) > 0 {
			// Group blocks by position for proper layout
			positionGroups := groupTextBlocksByPosition(params.ImageFiles[i].TextBlocks)

			// Process each position group
			for position, blocks := range positionGroups {
				if len(blocks) == 0 {
					continue
				}

				fe.logger.Debug("Processing text blocks group",
					slog.String("position", position),
					slog.Int("block_count", len(blocks)))

				// Process blocks in this position group
				for j, block := range blocks {
					// Process text content (replace variables)
					processedText := fe.textProcessor.ProcessTextContent(block.Text, params.PipelineContext)

					// Create a copy of the block with processed text
					blockWithProcessedText := block
					blockWithProcessedText.Text = processedText

					// For first block in group, use original position
					if j == 0 {
						textFilter := fe.textProcessor.BuildTextBlockWithAnimation(
							blockWithProcessedText,
							params.Width, params.Height,
							params.ImageDurations[i],
							true) // Use local timeline
						imgFilter += "," + textFilter
					} else {
						// For subsequent blocks, adjust position for stacking
						adjustedBlock := calculateAdjustedPosition(
							blockWithProcessedText,
							position, j,
							params.Width, params.Height)
						textFilter := fe.textProcessor.BuildTextBlockWithAnimation(
							adjustedBlock,
							params.Width, params.Height,
							params.ImageDurations[i],
							true) // Use local timeline
						imgFilter += "," + textFilter
					}
				}
			}
		}

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

func convertToFFmpegColor(color string) string {
	// Check if it's already in a format FFmpeg understands (named color or hex)
	if color == "" || color[0] == '#' || !strings.Contains(color, "(") {
		return color
	}

	// Check for rgba() format
	rgbaRegex := regexp.MustCompile(`rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([\d\.]+)\s*\)`)
	if matches := rgbaRegex.FindStringSubmatch(color); len(matches) == 5 {
		r, _ := strconv.Atoi(matches[1])
		g, _ := strconv.Atoi(matches[2])
		b, _ := strconv.Atoi(matches[3])
		a, _ := strconv.ParseFloat(matches[4], 64)

		// Convert alpha from 0-1 to 0-255
		alpha := int(a * 255)

		// Format as 0xRRGGBBAA for FFmpeg
		return fmt.Sprintf("0x%02x%02x%02x%02x", r, g, b, alpha)
	}

	// Check for rgb() format
	rgbRegex := regexp.MustCompile(`rgb\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	if matches := rgbRegex.FindStringSubmatch(color); len(matches) == 4 {
		r, _ := strconv.Atoi(matches[1])
		g, _ := strconv.Atoi(matches[2])
		b, _ := strconv.Atoi(matches[3])

		// Format as 0xRRGGBBFF for FFmpeg (fully opaque)
		return fmt.Sprintf("0x%02x%02x%02xff", r, g, b)
	}

	// If we couldn't parse it, return the original string
	// FFmpeg will show an error but we'll avoid crashing
	return color
}

// Generates zoompan parameters for Ken Burns effect.
func (fe *FFmpegExecutorImpl) getKenBurnsParameters(
	style string,
	intensity string,
	imageIndex int,
	duration float64,
	framerate int) string {

	// Convert duration to frames
	frames := int(math.Round(duration * float64(framerate)))

	// Set zoom speed based on intensity
	zoomSpeed := 0.001 // Default moderate
	switch intensity {
	case "subtle":
		zoomSpeed = 0.0005
	case "moderate":
		zoomSpeed = 0.001
	case "strong":
		zoomSpeed = 0.002
	}

	// Set pan speed based on intensity (pixels per frame)
	panSpeed := 0.8 // Default moderate
	switch intensity {
	case "subtle":
		panSpeed = 0.5
	case "moderate":
		panSpeed = 0.8
	case "strong":
		panSpeed = 1.2
	}

	// Determine effect direction based on style or random
	if style == "random" {
		styles := []string{"zoom_in", "zoom_out", "pan_left", "pan_right"}
		style = styles[imageIndex%len(styles)]
	}

	// Build the appropriate zoompan parameters
	switch style {
	case "zoom_in":
		return fmt.Sprintf("zoompan=z='min(1.0+%f*on,1.3)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'",
			zoomSpeed, frames)

	case "zoom_out":
		return fmt.Sprintf("zoompan=z='max(1.3-%f*on,1.0)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'",
			zoomSpeed, frames)

	case "pan_left":
		return fmt.Sprintf("zoompan=z='1.1':d=%d:x='if(lte(on,%d),0,min(iw-(iw/zoom),%f*on))':y='ih/2-(ih/zoom/2)'",
			frames, frames/10, panSpeed)

	case "pan_right":
		return fmt.Sprintf("zoompan=z='1.1':d=%d:x='if(lte(on,%d),iw-(iw/zoom),max(0,iw-(iw/zoom)-%f*on))':y='ih/2-(ih/zoom/2)'",
			frames, frames/10, panSpeed)

	default:
		// Default to zoom in if unknown style
		return fmt.Sprintf("zoompan=z='min(1.0+%f*on,1.3)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'",
			zoomSpeed, frames)
	}
}
