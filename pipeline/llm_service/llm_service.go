package llm_service

import (
	"context"
	"strconv"
)

type LLMService interface {
    CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error)
}


// Helper function to safely parse float values (same as in Gemini service)
func safeParseFloat(value interface{}, defaultValue float64) float64 {
    switch v := value.(type) {
    case float64:
        return v
    case string:
        if parsed, err := strconv.ParseFloat(v, 64); err == nil {
            return parsed
        }
    case int:
        return float64(v)
    case int64:
        return float64(v)
    }
    return defaultValue
}