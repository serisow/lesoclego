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
	// Split required steps, handling the case where it might be empty
	var requiredStepsList []string
	if s.RequiredSteps != "" {
		requiredStepsList = strings.Split(s.RequiredSteps, ",")
	}

	// Replace placeholders in the prompt with previous step outputs
	prompt := s.Prompt
	for _, requiredStep := range requiredStepsList {
		requiredStep = strings.TrimSpace(requiredStep)
		if requiredStep == "" {
			continue
		}
		if value, ok := pipelineContext.GetStepOutput(requiredStep); ok {
			placeholder := fmt.Sprintf("{%s}", requiredStep)
			prompt = strings.Replace(prompt, placeholder, fmt.Sprintf("%v", value), -1)
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

	// Set the output if StepOutputKey is provided
	if s.StepOutputKey != "" {
		pipelineContext.SetStepOutput(s.StepOutputKey, result)
	}

	// Store the response
	s.Response = result

	return nil
}

func (s *LLMStepImpl) GetType() string {
	return "llm_step"
}
