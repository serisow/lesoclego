package pipeline

import (
	"context"
	"fmt"

	"github.com/serisow/lesocle/pipeline/llm_service"
)

// Used essentially to detect if pipeline might run, so we fetch minimal data
type ScheduledPipeline struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ScheduledTime int64  `json:"scheduled_time"`
}

// The full pipeline data
type Pipeline struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	Steps         []PipelineStep `json:"steps"`
	ScheduledTime int64          `json:"scheduled_time"`
	LLMServices   map[string]llm_service.LLMService
	Context       *Context
}

type PipelineStep struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	Weight           int                    `json:"weight"`
	StepDescription  string                 `json:"step_description"`
	StepOutputKey    string                 `json:"step_output_key"`
    RequiredSteps    string                 `json:"required_steps"`
	LLMConfig        string                 `json:"llm_config,omitempty"`
	Prompt           string                 `json:"prompt,omitempty"`
	Response         string                 `json:"response,omitempty"`
	UUID             string                 `json:"uuid"`
	LLMServiceConfig map[string]interface{} `json:"llm_service,omitempty"`
	ActionConfig     string                 `json:"action_config,omitempty"`
}

func ExecutePipeline(p *Pipeline, registry *PluginRegistry) error {
	ctx := context.Background()
    if p.Context == nil {
        p.Context = NewContext()
    }

	for _, pipelineStep := range p.Steps {
		var step Step
		var err error

		switch pipelineStep.Type {
		case "llm_step":
			llmStep := &LLMStepImpl{PipelineStep: pipelineStep}

			serviceName, ok := pipelineStep.LLMServiceConfig["service_name"].(string)
			if !ok {
				return fmt.Errorf("service_name not found in llm_service configuration for step %s", pipelineStep.ID)
			}

			llmServiceInstance, ok := registry.GetLLMService(serviceName)
			if !ok {
				return fmt.Errorf("unknown LLM service: %s", serviceName)
			}

			llmStep.LLMServiceInstance = llmServiceInstance
			step = llmStep

		case "action_step":
			step = &ActionStepImpl{PipelineStep: pipelineStep}

		default:
			return fmt.Errorf("unknown step type: %s", pipelineStep.Type)
		}

		err = step.Execute(ctx, p.Context)
		if err != nil {
			return fmt.Errorf("error executing step %s: %w", pipelineStep.ID, err)
		}
	}

	return nil
}
