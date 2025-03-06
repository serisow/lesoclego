package video

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

// TextProcessorImpl implements the TextProcessor interface
type TextProcessorImpl struct {}

// NewTextProcessor creates a new text processor instance
func NewTextProcessor() TextProcessor {
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

// GetTextPosition calculates position parameters for FFmpeg drawtext filter
func (tp *TextProcessorImpl) GetTextPosition(position string, resolution string, customCoords map[string]string) string {
	// Default margin
	margin := 20

	// Default to center if invalid
	if position == "" {
		position = "center"
	}

	switch position {
	case "top":
		return fmt.Sprintf("x=(w-text_w)/2:y=%d", margin)
	case "bottom":
		return fmt.Sprintf("x=(w-text_w)/2:y=h-text_h-%d", margin)
	case "center":
		return "x=(w-text_w)/2:y=(h-text_h)/2"
	case "top_left":
		return fmt.Sprintf("x=%d:y=%d", margin, margin)
	case "top_right":
		return fmt.Sprintf("x=w-text_w-%d:y=%d", margin, margin)
	case "bottom_left":
		return fmt.Sprintf("x=%d:y=h-text_h-%d", margin, margin)
	case "bottom_right":
		return fmt.Sprintf("x=w-text_w-%d:y=h-text_h-%d", margin, margin)
	case "custom":
		// Use custom coordinates if available
		if customCoords != nil {
			x, xExists := customCoords["x"]
			y, yExists := customCoords["y"]
			if xExists && yExists {
				return fmt.Sprintf("x=%s:y=%s", x, y)
			}
		}
		// Fall back to center if custom coordinates are invalid
		return "x=(w-text_w)/2:y=(h-text_h)/2"
	default:
		// Default to bottom if position is not recognized
		return fmt.Sprintf("x=(w-text_w)/2:y=h-text_h-%d", margin)
	}
}

// BuildTextOverlayFilter generates the FFmpeg drawtext filter parameters
func (tp *TextProcessorImpl) BuildTextOverlayFilter(config map[string]interface{}, text string, position string) string {
	// Get defaults for required fields
	text = strings.ReplaceAll(text, ":", "\\:") // Escape colons
	text = strings.ReplaceAll(text, "'", "\\'") // Escape single quotes

	// Extract configuration values with defaults
	fontSize := "40"
	if fs, ok := config["font_size"].(string); ok && fs != "" {
		fontSize = fs
	}

	fontColor := "white"
	if fc, ok := config["font_color"].(string); ok && fc != "" {
		fontColor = fc
	}

	// Extract custom position coordinates
	customCoords := make(map[string]string)
	if cx, ok := config["custom_x"].(string); ok {
		customCoords["x"] = cx
	}
	if cy, ok := config["custom_y"].(string); ok {
		customCoords["y"] = cy
	}

	// Get position parameters
	positionParams := tp.GetTextPosition(position, "", customCoords)

	// Build the complete filter
	filter := fmt.Sprintf("drawtext=text='%s':fontsize=%s:fontcolor=%s:%s",
		text, fontSize, fontColor, positionParams)

	// Add background box if configured
	if backgroundColor, ok := config["background_color"].(string); ok && backgroundColor != "" {
		filter += fmt.Sprintf(":box=1:boxcolor=%s:boxborderw=5", backgroundColor)
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