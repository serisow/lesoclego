package video

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

// TextProcessorImpl implements the TextProcessor interface
type TextProcessorImpl struct {}

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

// EscapeFFmpegText escapes text for FFmpeg filter_complex parameter
func (tp *TextProcessorImpl) EscapeFFmpegText(text string) string {
	// First, escape backslashes
	text = strings.ReplaceAll(text, "\\", "\\\\")
	
	// Escape single quotes
	text = strings.ReplaceAll(text, "'", "\\'")
	
	// Escape other special characters that might break the filter
	specialChars := []string{":", ",", "[", "]", ";", "="}
	for _, char := range specialChars {
		text = strings.ReplaceAll(text, char, "\\"+char)
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