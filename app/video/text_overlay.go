package video

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

// TextProcessorImpl implements the TextProcessor interface
type TextProcessorImpl struct{}

// NewTextProcessor creates a new text processor instance
func NewTextProcessor() TextProcessor {
	// Return the concrete implementation, not a pointer to the interface
	return &TextProcessorImpl{}
}

// ProcessTextContent replaces placeholder variables in text overlay content
func (tp *TextProcessorImpl) ProcessTextContent(text string, pipelineContext *pipeline_type.Context) string {
	// Look for placeholders in the format {step_output_key}
	return regexp.MustCompile(`\{([^}]+)\}`).ReplaceAllStringFunc(text, func(placeholder string) string {
		// Extract key from placeholder
		key := strings.Trim(placeholder, "{}")

		// Look up the value in the context
		if value, exists := pipelineContext.GetStepOutput(key); exists {
			// Convert the value to string based on its type
			switch v := value.(type) {
			case string:
				return v
			case map[string]interface{}:
				// If it's a map, try to extract a meaningful value
				if textValue, ok := v["text"].(string); ok {
					return textValue
				}
				// Fallback to JSON representation
				if jsonBytes, err := json.Marshal(v); err == nil {
					return string(jsonBytes)
				}
			}
			// For other types, convert to string
			return fmt.Sprintf("%v", value)
		}

		// Return the original placeholder if key not found
		return placeholder
	})
}

// GetTextPosition determines position parameters for FFmpeg drawtext filter
func (tp *TextProcessorImpl) GetTextPosition(block TextBlock, width, height int) string {
	margin := 20 // Default margin

	if block.Position == "custom" && block.CustomX != "" && block.CustomY != "" {
		// For custom positions, use the exact coordinates
		return fmt.Sprintf("x=%s:y=%s", block.CustomX, block.CustomY)
	}

	switch block.Position {
	case "top_left":
		return fmt.Sprintf("x=%d:y=%d", margin, margin)
	case "top":
		return fmt.Sprintf("x=(w-text_w)/2:y=%d", margin)
	case "top_right":
		return fmt.Sprintf("x=w-text_w-%d:y=%d", margin, margin)
	case "left":
		return fmt.Sprintf("x=%d:y=(h-text_h)/2", margin)
	case "center":
		return "x=(w-text_w)/2:y=(h-text_h)/2"
	case "right":
		return fmt.Sprintf("x=w-text_w-%d:y=(h-text_h)/2", margin)
	case "bottom_left":
		return fmt.Sprintf("x=%d:y=h-text_h-%d", margin, margin)
	case "bottom":
		return fmt.Sprintf("x=(w-text_w)/2:y=h-text_h-%d", margin)
	case "bottom_right":
		return fmt.Sprintf("x=w-text_w-%d:y=h-text_h-%d", margin, margin)
	default:
		// Default to center if position is not recognized
		return "x=(w-text_w)/2:y=(h-text_h)/2"
	}
}

// Update the EscapeFFmpegText function in text_overlay.go
func (tp *TextProcessorImpl) EscapeFFmpegText(text string) string {
    // Replace any literal backslashes with QUADRUPLE backslashes
    // (this is because both shell and FFmpeg will interpret them)
    text = strings.ReplaceAll(text, "\\", "\\\\\\\\")
    // First escape single quotes for shell
    //text = strings.ReplaceAll(text, "'", "'\\''")
	text = strings.ReplaceAll(text, "'", "\\'")

	// Escape % symbol
	text = strings.ReplaceAll(text, "%","\\%" )
    
    // Define replacements map for FFmpeg filter_complex syntax
    replacements := map[string]string{
        // Special characters in filter_complex
		"'": "'\\''",
        ":" : "\\:",     // colon
        "," : "\\,",     // comma
        ";" : "\\;",     // semicolon
        "=" : "\\=",     // equals
        "[" : "\\[",     // brackets
        "]" : "\\]",
        "?" : "\\?",     // question mark
        "!" : "\\!",     // exclamation
        "#" : "\\#",     // hash
        "$" : "\\$",     // dollar
        "%" : "\\%",     // percentage
        "&" : "\\&",     // ampersand
        "(" : "\\(",     // parentheses
        ")" : "\\)",
        "*" : "\\*",     // asterisk
        "+" : "\\+",     // plus
        "/" : "\\/",     // slash
        "<" : "\\<",     // angle brackets
        ">" : "\\>",
        "@" : "\\@",     // at sign
        "^" : "\\^",     // caret
        "|" : "\\|",     // pipe
        "~" : "\\~",     // tilde
        "`" : "\\`",     // backtick
        "\"": "\\\"",    // double quote
        " " : "\\ ",     // Space (for shell safety)
        "{" : "\\{",     // Curly braces
        "}" : "\\}",
        "\t": "\\t",     // Tab character
        "." : "\\.",     // Dot
    }
    
    // Apply all replacements
    for original, escaped := range replacements {
        text = strings.ReplaceAll(text, original, escaped)
    }
    
    return text
}

// groupTextBlocksByPosition groups text blocks by their position
func groupTextBlocksByPosition(blocks []TextBlock) map[string][]TextBlock {
	groups := map[string][]TextBlock{
		"top_left":     {},
		"top":          {},
		"top_right":    {},
		"left":         {},
		"center":       {},
		"right":        {},
		"bottom_left":  {},
		"bottom":       {},
		"bottom_right": {},
		"custom":       {},
	}

	for _, block := range blocks {
		if block.Position == "custom" {
			groups["custom"] = append(groups["custom"], block)
		} else if _, exists := groups[block.Position]; exists {
			groups[block.Position] = append(groups[block.Position], block)
		} else {
			// Default to center for unknown positions
			groups["center"] = append(groups["center"], block)
		}
	}

	return groups
}

// calculateAdjustedPosition creates a new block with adjusted position for stacking
func calculateAdjustedPosition(block TextBlock, position string, index int, width, height int) TextBlock {
	adjustedBlock := block
	fontSize, _ := strconv.Atoi(block.FontSize)
	if fontSize == 0 {
		fontSize = 24
	}

	verticalSpacing := float64(fontSize) * 1.5
	margin := 20

	// Always use custom positioning for adjusted blocks
	adjustedBlock.Position = "custom"

	// Set horizontal position based on the original position group
	switch position {
	case "top_left", "left", "bottom_left":
		adjustedBlock.CustomX = fmt.Sprintf("%d", margin)
	case "top", "center", "bottom":
		adjustedBlock.CustomX = fmt.Sprintf("%d", width/2)
	case "top_right", "right", "bottom_right":
		adjustedBlock.CustomX = fmt.Sprintf("%d", width-margin)
	}

	// Set vertical position based on the group and index
	switch position {
	case "top_left", "top", "top_right":
		// Stack downward from top
		adjustedBlock.CustomY = fmt.Sprintf("%d", margin+int(float64(index)*verticalSpacing))
	case "left", "center", "right":
		// Alternate above and below center
		if index%2 == 0 {
			adjustedBlock.CustomY = fmt.Sprintf("%d", height/2+int(float64(index)*verticalSpacing/2))
		} else {
			adjustedBlock.CustomY = fmt.Sprintf("%d", height/2-int(float64(index)*verticalSpacing/2))
		}
	case "bottom_left", "bottom", "bottom_right":
		// Stack upward from bottom
		adjustedBlock.CustomY = fmt.Sprintf("%d", height-margin-int(float64(index)*verticalSpacing))
	}

	return adjustedBlock
}

// BuildTextOverlayFilter generates the FFmpeg drawtext filter parameters
func (tp *TextProcessorImpl) BuildTextBlockFilter(block TextBlock, width, height int) string {
	// Escape special characters
	text := tp.EscapeFFmpegText(block.Text)

	// Get position parameters
	positionParams := tp.GetTextPosition(block, width, height)

	// Build the basic filter
	filter := fmt.Sprintf("drawtext=text='%s':fontsize=%s:fontcolor=%s:%s", text, block.FontSize, convertToFFmpegColor(block.FontColor), positionParams)

	// Add background if specified
	if block.BackgroundColor != "" {
		filter += fmt.Sprintf(":box=1:boxcolor=%s:boxborderw=5", convertToFFmpegColor(block.BackgroundColor))
	}

	return filter
}

// ValidateTextOverlayConfig checks if the text overlay configuration is valid
func (tp *TextProcessorImpl) ValidateTextOverlayConfig(config map[string]interface{}) bool {
	// Check if enabled flag is present and true
	enabledValue, ok := config["enabled"]
	if !ok {
		return false
	}

	// Check enabled value based on type
	var enabled bool
	switch v := enabledValue.(type) {
	case string:
		enabled = v == "1" || strings.ToLower(v) == "true"
	case bool:
		enabled = v
	case float64:
		enabled = v == 1
	case int:
		enabled = v == 1
	default:
		enabled = false
	}

	if !enabled {
		return false
	}

	// Check if text is present and not empty
	text, ok := config["text"].(string)
	if !ok || text == "" {
		return false
	}

	return true
}

func (tp *TextProcessorImpl) getSlideDirectionFromPosition(position string) string {
	switch position {
	case "top", "top_left", "top_right":
		return "top"
	case "bottom", "bottom_left", "bottom_right":
		return "bottom"
	case "left":
		return "left"
	case "right":
		return "right"
	case "center":
	default:
		// For center position, default to bottom
		return "bottom"
	}
	return "unsupported_direcion"
}

// BuildTextBlockWithAnimation generates the FFmpeg drawtext filter parameters with font & style
func (tp *TextProcessorImpl) BuildTextBlockWithAnimation(
	block TextBlock,
	width, height int,
	slideDuration float64,
	localTimeline bool) string {

	// Process text content
	text := tp.EscapeFFmpegText(block.Text)

	// Get position parameters
	positionParams := tp.GetTextPosition(block, width, height)

	// Extract position coordinates for animation calculations
	var posX, posY int

	// Parse position for animations
	if block.Position == "custom" && block.CustomX != "" && block.CustomY != "" {
		if x, err := strconv.Atoi(block.CustomX); err == nil {
			posX = x
		}
		if y, err := strconv.Atoi(block.CustomY); err == nil {
			posY = y
		}
	} else {
		margin := 20
		switch block.Position {
		case "top_left":
			posX, posY = margin, margin
		case "top":
			posX, posY = width/2, margin
		case "top_right":
			posX, posY = width-margin, margin
		case "left":
			posX, posY = margin, height/2
		case "center":
			posX, posY = width/2, height/2
		case "right":
			posX, posY = width-margin, height/2
		case "bottom_left":
			posX, posY = margin, height-margin
		case "bottom_right":
			posX, posY = width-margin, height-margin
		case "bottom":
			posX, posY = width/2, height-margin
		}
	}

	// Parse font size for scale animation
	fontSize := 24 // Default
	if size, err := strconv.Atoi(block.FontSize); err == nil {
		fontSize = size
	}

	// Get font parameters for font family
	fontParam := ""
	if block.FontFamily != "" && block.FontFamily != "default" {
		fontFile := tp.getFontFilePath(block.FontFamily)
		if fontFile != "" {
			fontParam = fmt.Sprintf(":fontfile='%s'", fontFile)
		}
	}

	// Get style parameters based on font style
	styleParams := ""
	switch block.FontStyle {
	case "outline":
		styleParams = ":borderw=1.5:bordercolor=black"
	case "shadow":
		styleParams = ":shadowx=2:shadowy=2:shadowcolor=black"
	case "outline_shadow":
		styleParams = ":borderw=1.5:bordercolor=black:shadowx=2:shadowy=2:shadowcolor=black"
	case "normal":
	default:
		// No additional styling
	}

	// Build the basic filter
	filter := fmt.Sprintf("drawtext=text='%s':fontsize=%s:fontcolor=%s%s%s:%s",
		text, block.FontSize, convertToFFmpegColor(block.FontColor), fontParam, styleParams, positionParams)

	// Add background if specified
	if block.BackgroundColor != "" {
		filter += fmt.Sprintf(":box=1:boxcolor=%s:boxborderw=5",
			convertToFFmpegColor(block.BackgroundColor))
	}

	// Default animation values
	animationType := "none"
	animationDuration := 1.0
	animationDelay := 0.0

	// Extract animation parameters if present
	if block.Animation != nil {
		animationType = block.Animation.Type
		animationDuration = block.Animation.Duration
		animationDelay = block.Animation.Delay
	}

	// Ensure animation duration isn't too long
	if animationDuration > slideDuration/2 {
		animationDuration = slideDuration / 2
	}

	// Animation timing calculations
	fadeInStart := animationDelay
	fadeInEnd := fadeInStart + animationDuration
	fadeOutStart := slideDuration - animationDuration
	fadeOutEnd := slideDuration

	// Make sure animation stays within slide boundaries
	if fadeInEnd > slideDuration {
		fadeInEnd = slideDuration
	}
	if fadeOutStart < fadeInEnd {
		fadeOutStart = fadeInEnd
	}

	// Create enable expression for the full duration of this slide
	filter += fmt.Sprintf(":enable='between(t,0,%.3f)'", slideDuration)

	// Apply animation based on type
	switch animationType {
	case "fade":
		// Fade in at start, fade out at end
		filter += fmt.Sprintf(":alpha='if(between(t,%.3f,%.3f),(t-%.3f)/%.3f,if(between(t,%.3f,%.3f),1-(t-%.3f)/%.3f,1))'",
			fadeInStart, fadeInEnd, // Fade in range
			fadeInStart, animationDuration, // Fade in calculation
			fadeOutStart, fadeOutEnd, // Fade out range
			fadeOutStart, animationDuration) // Fade out calculation

	case "slide":
		// Determine slide direction based on position
		slideDirection := tp.getSlideDirectionFromPosition(block.Position)
		slideOffset := 100

		if slideDirection == "left" || slideDirection == "right" {
			if slideDirection == "left" {
				// Slide from left
				filter += fmt.Sprintf(":x='if(between(t,%.3f,%.3f),%.3f+(%.3f*(t-%.3f)/%.3f),%.3f)'",
					fadeInStart, fadeInEnd, // Time range
					float64(posX-slideOffset),                            // Starting position
					float64(slideOffset), fadeInStart, animationDuration, // Movement calculation
					float64(posX)) // Final position
			} else {
				// Slide from right
				filter += fmt.Sprintf(":x='if(between(t,%.3f,%.3f),%.3f-(%.3f*(t-%.3f)/%.3f),%.3f)'",
					fadeInStart, fadeInEnd, // Time range
					float64(posX+slideOffset),                            // Starting position
					float64(slideOffset), fadeInStart, animationDuration, // Movement calculation
					float64(posX)) // Final position
			}
		} else {
			if slideDirection == "top" {
				// Slide from top
				filter += fmt.Sprintf(":y='if(between(t,%.3f,%.3f),%.3f+(%.3f*(t-%.3f)/%.3f),%.3f)'",
					fadeInStart, fadeInEnd, // Time range
					float64(posY-slideOffset),                            // Starting position
					float64(slideOffset), fadeInStart, animationDuration, // Movement calculation
					float64(posY)) // Final position
			} else {
				// Slide from bottom
				filter += fmt.Sprintf(":y='if(between(t,%.3f,%.3f),%.3f-(%.3f*(t-%.3f)/%.3f),%.3f)'",
					fadeInStart, fadeInEnd, // Time range
					float64(posY+slideOffset),                            // Starting position
					float64(slideOffset), fadeInStart, animationDuration, // Movement calculation
					float64(posY)) // Final position
			}
		}

		// Add fade in for smoother appearance
		filter += fmt.Sprintf(":alpha='if(between(t,%.3f,%.3f),(t-%.3f)/%.3f,1)'",
			fadeInStart, fadeInEnd, // Range
			fadeInStart, animationDuration) // Fade calculation

	case "scale":
		// Scale animation
		filter += fmt.Sprintf(":fontsize='if(between(t,%.3f,%.3f),%.3f*(t-%.3f)/%.3f,%.3f)'",
			fadeInStart, fadeInEnd, // Time range
			float64(fontSize),              // Target size
			fadeInStart, animationDuration, // Scaling calculation
			float64(fontSize)) // Final size

		// Fade in for smoother appearance
		filter += fmt.Sprintf(":alpha='if(between(t,%.3f,%.3f),(t-%.3f)/%.3f,1)'",
			fadeInStart, fadeInEnd, // Range
			fadeInStart, animationDuration) // Fade calculation

	case "typewriter":
		// For typewriter effect, we need special handling for newlines and proper escaping
		// Sanitize problematic characters for ffmpeg
		safeText := strings.NewReplacer(
			"@", "[at]",
			":", " ",
			";", " ",
			",", " ",
			"\\", "").Replace(block.Text)

		// Create a new escaped version of the sanitized text
		escapedText := tp.EscapeFFmpegText(safeText)

		// Update the filter with the new escaped text
		filter = strings.Replace(filter,
			fmt.Sprintf("text='%s'", text),
			fmt.Sprintf("text='%s'", escapedText), 1)

		// Just use a simple fade-in animation like the PHP implementation
		filter += fmt.Sprintf(":alpha='if(between(t,%.3f,%.3f),(t-%.3f)/%.3f,1)'",
			fadeInStart, fadeInEnd,
			fadeInStart, animationDuration)
	}

	return filter
}

// getFontFilePath attempts to find a font file based on the provided font family name
func (tp *TextProcessorImpl) getFontFilePath(fontFamily string) string {
	// Common font directories to search
	fontDirs := []string{
		"/usr/share/fonts/dejavu",
		"/usr/share/fonts/opensans",
		"/usr/share/fonts/droid",
		"/usr/share/fonts/liberation",
		"/usr/share/fonts/freefont",
		"/usr/share/fonts/truetype",
		"/usr/share/fonts/TTF",
	}

	// Common font filename patterns for the requested font family
	fontVariants := []string{
		fontFamily + ".ttf",
		fontFamily + "-Regular.ttf",
		strings.ToLower(fontFamily) + ".ttf",
		strings.ToLower(fontFamily) + "-regular.ttf",
		strings.ReplaceAll(fontFamily, " ", "") + ".ttf",
		strings.ReplaceAll(strings.ToLower(fontFamily), " ", "") + ".ttf",
	}

	// Search for the font file in all font directories
	for _, dir := range fontDirs {
		for _, variant := range fontVariants {
			fontPath := filepath.Join(dir, variant)
			if _, err := os.Stat(fontPath); err == nil {
				// Font file found
				return fontPath
			}
		}
	}

	// If we get here, try a more exhaustive search
	for _, dir := range fontDirs {
		if _, err := os.Stat(dir); err == nil {
			// Directory exists, search recursively
			matches, err := filepath.Glob(filepath.Join(dir, "**", "*.ttf"))
			if err != nil {
				// Fall back to a simpler direct search
				matches, _ = filepath.Glob(filepath.Join(dir, "*.ttf"))
			}

			// Look for a font name match
			for _, match := range matches {
				baseName := strings.ToLower(filepath.Base(match))
				searchName := strings.ToLower(fontFamily)
				
				// Check if the font file name contains the requested font family name
				if strings.Contains(baseName, searchName) {
					return match
				}
			}
		}
	}

	// Font not found, return empty string
	return ""
}