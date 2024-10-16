package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/serisow/lesocle/pipeline/llm_service"
)

type LLMStepImpl struct {
	PipelineStep
	LLMServiceInstance llm_service.LLMService
}

func (s *LLMStepImpl) Execute(ctx context.Context, pipelineContext *Context) error {
    // Split required steps
    requiredSteps := strings.Split(s.RequiredSteps, "\r\n")

    // Replace placeholders in the prompt with previous step outputs
    prompt := s.Prompt
    for _, requiredStep := range requiredSteps {
        requiredStep = strings.TrimSpace(requiredStep)
        if requiredStep == "" {
            continue
        }
        if value, ok := pipelineContext.GetStepOutput(requiredStep); ok {
            placeholder := fmt.Sprintf("{%s}", requiredStep)
            prompt = strings.Replace(prompt, placeholder, fmt.Sprintf("%v", value), -1)
        } else {
            return fmt.Errorf("required step output '%s' not found in context", requiredStep)
        }
    }
	// Ensure LLMService is not nil
	if s.LLMServiceInstance == nil {
		return fmt.Errorf("LLMService is not initialized for step %s", s.ID)
	}

	// Call the LLM service
	result, err := s.LLMServiceInstance.CallLLM(ctx, s.LLMServiceConfig, prompt)
	if err != nil {
		return fmt.Errorf("error calling LLM service for step %s: %w", s.ID, err)
	}

    if s.StepOutputKey != "" {
        pipelineContext.SetStepOutput(s.StepOutputKey, result)
    }
	return nil
}

func (s *LLMStepImpl) GetType() string {
	return "llm_step"
}
