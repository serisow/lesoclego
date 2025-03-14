package video

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

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

// getStringValue gets a string value from a map with a default fallback
func getStringValue(config map[string]interface{}, key string, defaultValue string) string {
    // First check if it's a string
    if val, ok := config[key].(string); ok && val != "" {
        return val
    }
    
    // Check if it's a number (float64 is the default for JSON numbers)
    if val, ok := config[key].(float64); ok {
        return fmt.Sprintf("%g", val)  // %g will remove trailing zeros
    }
    
    // Check if it's an integer
    if val, ok := config[key].(int); ok {
        return fmt.Sprintf("%d", val)
    }
    
    // If not found or not a recognizable type, return default
    return defaultValue
}

// getStepIDs extracts step IDs for logging
func getStepIDs(steps []pipeline_type.PipelineStep) []string {
	ids := make([]string, len(steps))
	for i, step := range steps {
		ids[i] = step.ID
	}
	return ids
}

// Add this function to ffmpeg.go:
func getBoolValue(m map[string]interface{}, key string, defaultValue bool) (bool, bool) {
    switch v := m[key].(type) {
    case bool:
        return v, true
    case string:
        if v == "1" || strings.ToLower(v) == "true" {
            return true, true
        } else if v == "0" || strings.ToLower(v) == "false" {
            return false, true
        }
    case float64:
        if v == 1.0 {
            return true, true
        } else if v == 0.0 {
            return false, true
        }
    case int:
        if v == 1 {
            return true, true
        } else if v == 0 {
            return false, true
        }
    }
    return defaultValue, false
}