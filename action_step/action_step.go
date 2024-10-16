package action_step

import (
	"context"
	"fmt"

	"github.com/serisow/lesocle/services/action_service"
	"github.com/serisow/lesocle/pipeline_type"
)

type ActionStepImpl struct {
    PipelineStep          pipeline_type.PipelineStep
    ActionServiceInstance action_service.ActionService
}

func (s *ActionStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    // Ensure ActionServiceInstance is not nil
    if s.ActionServiceInstance == nil {
        return fmt.Errorf("ActionService is not initialized for step %s", s.PipelineStep.ID)
    }

    // Call the action service
    result, err := s.ActionServiceInstance.Execute(ctx, s.PipelineStep.ActionConfig, pipelineContext, &s.PipelineStep)
    if err != nil {
        return fmt.Errorf("error executing action service for step %s: %w", s.PipelineStep.ID, err)
    }

    // Store the result in the pipeline context
    if s.PipelineStep.StepOutputKey != "" {
        pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, result)
    }

    return nil
}

func (s *ActionStepImpl) GetType() string {
    return "action_step"
}
