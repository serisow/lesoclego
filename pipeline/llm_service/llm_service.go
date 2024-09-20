package llm_service

import "context"

type LLMService interface {
    CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error)
}