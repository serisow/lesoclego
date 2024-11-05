package llm_step

import (
	"context"
	"fmt"
	"strings"

	"github.com/serisow/lesocle/services/llm_service"
	"github.com/serisow/lesocle/pipeline_type"
)

type LLMStepImpl struct {
    PipelineStep       pipeline_type.PipelineStep
	LLMServiceInstance llm_service.LLMService
}

func (s *LLMStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    // Split required steps
    requiredSteps := strings.Split(s.PipelineStep.RequiredSteps, "\r\n")

    // Replace placeholders in the prompt with previous step outputs
    prompt := s.PipelineStep.Prompt
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
		return fmt.Errorf("LLMService is not initialized for step %s", s.PipelineStep.ID)
	}

	// Call the LLM service
	result, err := s.LLMServiceInstance.CallLLM(ctx, s.PipelineStep.LLMServiceConfig, prompt)
	if err != nil {
		return fmt.Errorf("error calling LLM service for step %s: %w", s.PipelineStep.ID, err)
	}

    if s.PipelineStep.StepOutputKey != "" {
        pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, result)
    }
	return nil
}

func (s *LLMStepImpl) GetType() string {
	return "llm_step"
}
