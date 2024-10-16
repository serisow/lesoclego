package llm_service

import (
    "context"
)

type MockLLMService struct {
    CallLLMFunc func(ctx context.Context, config map[string]interface{}, prompt string) (string, error)
}

func (m *MockLLMService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    if m.CallLLMFunc != nil {
        return m.CallLLMFunc(ctx, config, prompt)
    }
    return "mock response", nil
}
