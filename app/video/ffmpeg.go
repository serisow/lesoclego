package video

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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

// GetResolutionWithOrientation returns the resolution based on quality and orientation
func (fe *FFmpegExecutorImpl) GetResolutionWithOrientation(quality string, vertical bool) string {
	// Standard horizontal resolutions
	resolutions := map[string]string{
		"low":    "640:480",
		"medium": "1280:720",
		"high":   "1920:1080",
	}

	// Vertical resolutions for social media
	verticalResolutions := map[string]string{
		"low":    "480:640",
		"medium": "720:1280",
		"high":   "1080:1920", // Standard 9:16 for TikTok/Instagram
	}

	defaultResolution := "1280:720"         // Default horizontal
	defaultVerticalResolution := "720:1280" // Default vertical

	if vertical {
		if res, ok := verticalResolutions[quality]; ok {
			return res
		}
		return defaultVerticalResolution
	}

	if res, ok := resolutions[quality]; ok {
		return res
	}
	return defaultResolution
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

	// --- NEW: Determine orientation ---
	orientation := getStringValue(params.Config, "orientation", "horizontal")
	isVertical := orientation == "vertical"
	// --- END NEW ---

	// Get transition configuration
	transitionType := getStringValue(params.Config, "transition_type", "fade")
	transitionDurationStr := getStringValue(params.Config, "transition_duration", "1")
	transitionDuration, err := strconv.ParseFloat(transitionDurationStr, 64)
	if err != nil {
		transitionDuration = 1.0
		fe.logger.Warn("Failed to parse transition_duration, using default",
			slog.String("value", transitionDurationStr),
			slog.Float64("default", transitionDuration))
	}

	// --- UPDATED: Use new GetResolution ---
	videoQuality := getStringValue(params.Config, "video_quality", "medium")
	if isVertical {
		params.Resolution = fe.GetResolutionWithOrientation(videoQuality, true)
	} else {
		params.Resolution = fe.GetResolution(videoQuality)
	}
	// --- END UPDATED ---

	// Parse resolution to get width and height (should happen *after* getting resolution)
	parts := strings.Split(params.Resolution, ":")
	if len(parts) == 2 {
		if w, err := strconv.Atoi(parts[0]); err == nil {
			params.Width = w
		}
		if h, err := strconv.Atoi(parts[1]); err == nil {
			params.Height = h
		}
	}
	// Add a check if width/height failed parsing (use defaults?)
	if params.Width == 0 || params.Height == 0 {
		fe.logger.Error("Failed to parse resolution, using default 1280x720", slog.String("resolution", params.Resolution))
		// Apply a default based on orientation to avoid division by zero later
		if isVertical {
			params.Width = 720
			params.Height = 1280
			params.Resolution = "720:1280"
		} else {
			params.Width = 1280
			params.Height = 720
			params.Resolution = "1280:720"
		}
	}

	// Check Ken Burns configuration (same as before)
	kenBurnsEnabled := false
	kenBurnsStyle := "random"
	kenBurnsIntensity := "moderate"
	if kenBurnsConfig, ok := params.Config["ken_burns"].(map[string]interface{}); ok {
		if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(float64); ok {
			kenBurnsEnabled = enabled == 1
		} else if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(bool); ok {
			kenBurnsEnabled = enabled
		} else if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(string); ok {
			kenBurnsEnabled = enabled == "1" || strings.ToLower(enabled) == "true"
		} else if enabled, ok := kenBurnsConfig["ken_burns_enabled"].(int); ok {
			kenBurnsEnabled = enabled == 1
		}
		if style, ok := kenBurnsConfig["ken_burns_style"].(string); ok && style != "" {
			kenBurnsStyle = style
		}
		if intensity, ok := kenBurnsConfig["ken_burns_intensity"].(string); ok && intensity != "" {
			kenBurnsIntensity = intensity
		}
	}

	// Build ffmpeg command args (same init)
	args := []string{}
	for _, imagePath := range imagePaths {
		args = append(args, "-loop", "1", "-i", imagePath)
	}
	args = append(args, "-i", params.AudioFile.URI)

	// Create filter complex string
	filterComplex := ""

	// Get framerate (needed for Ken Burns)
	framerate := 24 // Default
	if frameRateStr, ok := params.Config["framerate"].(string); ok {
		if fr, err := strconv.Atoi(frameRateStr); err == nil && fr > 0 {
			framerate = fr
		}
	} else if frameRateFloat, ok := params.Config["framerate"].(float64); ok && frameRateFloat > 0 {
		framerate = int(frameRateFloat)
	} else if frameRateInt, ok := params.Config["framerate"].(int); ok && frameRateInt > 0 {
		framerate = frameRateInt
	} else if frameRateInt, ok := params.Config["framerate"].(int64); ok && frameRateInt > 0 {
		framerate = int(frameRateInt)
	}

	// Process each image - scale, crop, Ken Burns, text blocks
	for i := 0; i < len(imagePaths); i++ {
		// --- UPDATED: Replicate PHP's buildImageFiltersWithText logic ---
		// Start filter chain for this image input
		imgFilter := fmt.Sprintf("[%d:v]", i)

		// 1. Scale to COVER target WxH, maintaining aspect ratio
		imgFilter += fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase:force_divisible_by=2",
			params.Width, params.Height)

		// 2. Crop scaled image back to target WxH (center crop)
		imgFilter += fmt.Sprintf(",crop=%d:%d:(iw-ow)/2:(ih-oh)/2",
			params.Width, params.Height)

		// 3. Apply Ken Burns effect AFTER cropping (if enabled)
		if kenBurnsEnabled {
			kenBurnsFilter := fe.getKenBurnsParameters(
				kenBurnsStyle,
				kenBurnsIntensity,
				i,
				params.ImageDurations[i],
				framerate,
				params.Width,  // <-- Pass target width
				params.Height, // <-- Pass target height
			)
			imgFilter += "," + kenBurnsFilter
		}

		// 4. Set SAR and Pixel Format
		imgFilter += ",setsar=1,format=yuv420p"
		// --- END UPDATED ---

		// --- START: Equivalent of addTextOverlays ---
		// Add text blocks if available for this image
		// This logic remains structurally similar, but happens AFTER scale/crop/KB
		if i < len(params.ImageFiles) && len(params.ImageFiles[i].TextBlocks) > 0 {
			positionGroups := groupTextBlocksByPosition(params.ImageFiles[i].TextBlocks)
			for position, blocks := range positionGroups {
				if len(blocks) == 0 {
					continue
				}

				fe.logger.Debug("Processing text blocks group",
					slog.String("position", position),
					slog.Int("block_count", len(blocks)))

				for j, block := range blocks {
					processedText := fe.textProcessor.ProcessTextContent(block.Text, params.PipelineContext)
					blockWithProcessedText := block // Make a copy
					blockWithProcessedText.Text = processedText

					var textFilter string
					if j == 0 {
						textFilter = fe.textProcessor.BuildTextBlockWithAnimation(
							blockWithProcessedText,
							params.Width, params.Height,
							params.ImageDurations[i],
							true) // Use local timeline
					} else {
						adjustedBlock := calculateAdjustedPosition(
							blockWithProcessedText,
							position, j,
							params.Width, params.Height)
						textFilter = fe.textProcessor.BuildTextBlockWithAnimation(
							adjustedBlock,
							params.Width, params.Height,
							params.ImageDurations[i],
							true) // Use local timeline
					}
					// Append text filter to the image filter chain
					imgFilter += "," + textFilter
				}
			}
		}
		// --- END: Equivalent of addTextOverlays ---

		// Label the output of this image's filter chain
		imgFilter += fmt.Sprintf("[v%d]", i)

		// Add to the overall filter complex
		if filterComplex != "" {
			filterComplex += ";"
		}
		filterComplex += imgFilter
	}

	// Add trimming for each image (remains the same)
	for i := 0; i < len(imagePaths); i++ {
		if filterComplex != "" {
			filterComplex += ";"
		}
		// Ensure duration is positive to avoid issues
		trimDuration := math.Max(0.001, params.ImageDurations[i])
		filterComplex += fmt.Sprintf("[v%d]trim=duration=%.3f,setpts=PTS-STARTPTS[hold%d]",
			i, trimDuration, i)
	}

	// Handle single image vs multiple images with transitions (remains the same)
	if len(imagePaths) == 1 {
		args = append(args, "-filter_complex", filterComplex)
		args = append(args, "-map", "[hold0]", "-map", fmt.Sprintf("%d:a", len(imagePaths)))
	} else {
		// Generate transitions between images
		currentOffset := math.Max(0.0, params.ImageDurations[0]-transitionDuration) // Ensure non-negative start offset
		lastOutput := "hold0"

		for i := 1; i < len(imagePaths); i++ {
			// Ensure offset and duration are not negative
			safeOffset := math.Max(0.0, currentOffset)
			safeTransDuration := math.Max(0.001, transitionDuration) // Minimum 1ms transition

			if filterComplex != "" {
				filterComplex += ";"
			}
			filterComplex += fmt.Sprintf("[%s][hold%d]xfade=transition=%s:duration=%.3f:offset=%.3f[trans%d]",
				lastOutput, i, transitionType, safeTransDuration, safeOffset, i)

			lastOutput = fmt.Sprintf("trans%d", i)

			// Update offset for next transition, ensuring non-negative image duration
			nextImageDuration := math.Max(0.001, params.ImageDurations[i])
			currentOffset += nextImageDuration - safeTransDuration
		}

		args = append(args, "-filter_complex", filterComplex)
		args = append(args, "-map", fmt.Sprintf("[%s]", lastOutput), "-map", fmt.Sprintf("%d:a", len(imagePaths)))
	}

	// Add encoding parameters (remains the same)
	args = append(args,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-pix_fmt", "yuv420p")

	// Add bitrate if specified (remains the same)
	if bitrate, ok := params.Config["bitrate"].(string); ok && bitrate != "" {
		args = append(args, "-b:v", bitrate)
	} else {
		// Consider adding a default bitrate if none is specified
		args = append(args, "-b:v", "1500k") // Example default
	}

	// Add framerate (using the value determined earlier)
	args = append(args, "-r", fmt.Sprintf("%d", framerate))

	// Add shortest flag (remains the same)
	args = append(args, "-shortest")

	// Add output file (remains the same)
	args = append(args, "-y", params.OutputPath)

	// Log the command (remains the same)
	fe.logger.Debug("Executing FFmpeg command", slog.Any("args", args))
	fe.logFFmpegCommand(args, filterComplex, nil) // Log before execution attempt

	// Execute command (remains the same)
	cmd := exec.Command("ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		// Log command even on pipe error
		fe.logFFmpegCommand(args, filterComplex, fmt.Errorf("stderr pipe error: %w", err))
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		// Log command even on start error
		fe.logFFmpegCommand(args, filterComplex, fmt.Errorf("start error: %w", err))
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	stderrOutput, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		fe.logger.Error("FFmpeg execution failed",
			slog.String("error", err.Error()),
			slog.String("stderr", string(stderrOutput)))
		// Log command and error on wait error
		fe.logFFmpegCommand(args, filterComplex, fmt.Errorf("wait error: %w\nStderr: %s", err, string(stderrOutput)))
		return fmt.Errorf("FFmpeg execution failed: %w", err)
	}

	if _, err := os.Stat(params.OutputPath); os.IsNotExist(err) {
		// Log command if output is missing
		fe.logFFmpegCommand(args, filterComplex, fmt.Errorf("output file missing\nStderr: %s", string(stderrOutput)))
		fe.logger.Error("FFmpeg finished but output file is missing",
			slog.String("path", params.OutputPath),
			slog.String("stderr", string(stderrOutput)))
		return fmt.Errorf("FFmpeg did not create an output file at %s", params.OutputPath)
	}

	fe.logger.Info("FFmpeg execution successful", slog.String("output", params.OutputPath))
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
	framerate int,
	targetWidth int, // <-- ADDED
	targetHeight int, // <-- ADDED
) string {

	// Convert duration to frames, ensure at least 1 frame
	frames := int(math.Max(1.0, math.Round(duration*float64(framerate))))

	// --- UPDATED: Zoom speed based on PHP ---
	zoomSpeedMap := map[string]float64{
		"subtle":   0.0005,
		"moderate": 0.001,
		"strong":   0.002,
	}
	zoomSpeed, ok := zoomSpeedMap[intensity]
	if !ok {
		zoomSpeed = 0.001 // Default moderate
	}

	// --- UPDATED: Pan speed based on PHP ---
	minDim := math.Min(float64(targetWidth), float64(targetHeight))
	panSpeedFactorMap := map[string]float64{
		"subtle":   0.05,
		"moderate": 0.1,
		"strong":   0.15,
	}
	panSpeedFactor, ok := panSpeedFactorMap[intensity]
	if !ok {
		panSpeedFactor = 0.1 // Default moderate
	}
	panSpeedPixelsPerFrame := (minDim * panSpeedFactor) / float64(framerate)
	// Ensure pan speed is at least a very small positive number if calculated as zero
	if panSpeedPixelsPerFrame <= 0 {
		panSpeedPixelsPerFrame = 0.01
	}

	// --- UPDATED: Random style selection ---
	if style == "random" {
		// Include new pan directions
		styles := []string{"zoom_in", "zoom_out", "pan_left", "pan_right", "pan_up", "pan_down"}
		// Simple pseudo-random based on index and duration factor
		styleIndex := (imageIndex + int(duration*10)) % len(styles)
		style = styles[styleIndex]
	}

	// --- NEW: Define output size parameter ---
	// CRITICAL: This tells zoompan the target dimensions of the frame it should render onto.
	outputSize := fmt.Sprintf(":s=%dx%d", targetWidth, targetHeight)

	// Build the appropriate zoompan parameters, matching PHP logic
	switch style {
	case "zoom_in":
		// Zoom in from 1.0 up to 1.2 (matches PHP default max zoom)
		// Note: using 'on' (output frame number) instead of 'zoom' variable for simplicity here
		// 'zoom' would require 'eval=frame' which is default, but 'on' is clearer for linear change.
		// 'zoom,1.0' is the initial zoom value.
		return fmt.Sprintf("zoompan=z='min(max(zoom,1.0)+%f,1.2)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'%s",
			zoomSpeed, frames, outputSize) // Append outputSize

	case "zoom_out":
		// Zoom out from 1.2 down to 1.0
		// Start at 1.2, decrease by zoomSpeed each frame ('on' starts at 0)
		return fmt.Sprintf("zoompan=z='max(1.2-%f*on,1.0)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'%s",
			zoomSpeed, frames, outputSize) // Append outputSize

	case "pan_left":
		// Start right (x=iw-iw/zoom), move left. Use z=1.1
		// Ensure x doesn't go below 0.
		startX := "max(0,iw-iw/1.1)" // Using z=1.1
		moveX := fmt.Sprintf("%f*on", panSpeedPixelsPerFrame)
		return fmt.Sprintf("zoompan=z=1.1:d=%d:x='max(0,%s-%s)':y='ih/2-(ih/1.1/2)'%s", // Use z=1.1 in y calc too
			frames, startX, moveX, outputSize) // Append outputSize

	case "pan_right":
		// Start left (x=0), move right. Use z=1.1
		// Ensure x doesn't exceed iw-iw/zoom.
		startX := "0"
		maxX := "iw-iw/1.1" // Max x offset for z=1.1
		moveX := fmt.Sprintf("%f*on", panSpeedPixelsPerFrame)
		return fmt.Sprintf("zoompan=z=1.1:d=%d:x='min(%s,%s+%s)':y='ih/2-(ih/1.1/2)'%s",
			frames, maxX, startX, moveX, outputSize) // Append outputSize

	case "pan_up":
		// Start bottom (y=ih-ih/zoom), move up. Use z=1.1
		startY := "max(0,ih-ih/1.1)"
		moveY := fmt.Sprintf("%f*on", panSpeedPixelsPerFrame)
		return fmt.Sprintf("zoompan=z=1.1:d=%d:x='iw/2-(iw/1.1/2)':y='max(0,%s-%s)'%s",
			frames, startY, moveY, outputSize) // Append outputSize

	case "pan_down":
		// Start top (y=0), move down. Use z=1.1
		startY := "0"
		maxY := "ih-ih/1.1"
		moveY := fmt.Sprintf("%f*on", panSpeedPixelsPerFrame)
		return fmt.Sprintf("zoompan=z=1.1:d=%d:x='iw/2-(iw/1.1/2)':y='min(%s,%s+%s)'%s",
			frames, maxY, startY, moveY, outputSize) // Append outputSize

	default:
		// Default to zoom in if unknown style (matches PHP)
		fe.logger.Warn("Unknown Ken Burns style, defaulting to zoom_in", slog.String("style", style))
		return fmt.Sprintf("zoompan=z='min(max(zoom,1.0)+%f,1.2)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'%s",
			zoomSpeed, frames, outputSize) // Append outputSize
	}
}

// logFFmpegCommand logs the complete FFmpeg command and any errors to files for debugging
func (fe *FFmpegExecutorImpl) logFFmpegCommand(args []string, filterComplex string, err error) {
	// Create logs directory if it doesn't exist
	logsDir := filepath.Join("logs", "ffmpeg")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		fe.logger.Error("Failed to create FFmpeg logs directory",
			slog.String("error", err.Error()))
		return
	}

	timestamp := time.Now().Format("20060102_150405")

	// Write the filter complex to a file
	filterPath := filepath.Join(logsDir, fmt.Sprintf("filter_%s.txt", timestamp))
	if err := os.WriteFile(filterPath, []byte(filterComplex), 0644); err != nil {
		fe.logger.Error("Failed to write filter complex to file",
			slog.String("error", err.Error()))
	} else {
		fe.logger.Info("Filter complex written to file for debugging",
			slog.String("path", filterPath))
	}

	// Write the complete command to a file
	cmdPath := filepath.Join(logsDir, fmt.Sprintf("command_%s.txt", timestamp))
	cmdContent := fmt.Sprintf("ffmpeg %s\n\nFull filter_complex:\n%s",
		strings.Join(args, " "), filterComplex)
	if err := os.WriteFile(cmdPath, []byte(cmdContent), 0644); err != nil {
		fe.logger.Error("Failed to write command to file",
			slog.String("error", err.Error()))
	} else {
		fe.logger.Info("FFmpeg command written to file for debugging",
			slog.String("path", cmdPath))
	}

	// If there was an error, log it too
	if err != nil {
		errorPath := filepath.Join(logsDir, fmt.Sprintf("error_%s.txt", timestamp))
		errorContent := fmt.Sprintf("Error: %v\n\nCommand: ffmpeg %s\n\nFilter Complex:\n%s",
			err, strings.Join(args, " "), filterComplex)
		if err := os.WriteFile(errorPath, []byte(errorContent), 0644); err != nil {
			fe.logger.Error("Failed to write error to file",
				slog.String("error", err.Error()))
		} else {
			fe.logger.Info("FFmpeg error written to file for debugging",
				slog.String("path", errorPath))
		}
	}
}
