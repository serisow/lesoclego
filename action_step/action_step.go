package action_step

import (
	"context"
	"fmt"

	"github.com/serisow/lesocle/action_service"
	"github.com/serisow/lesocle/pipeline_type"
)

type ActionStepImpl struct {
    pipeline_type.PipelineStep
    ActionServiceInstance action_service.ActionService
}

func (s *ActionStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    // Ensure ActionServiceInstance is not nil
    if s.ActionServiceInstance == nil {
        return fmt.Errorf("ActionService is not initialized for step %s", s.ID)
    }

    // Call the action service
    result, err := s.ActionServiceInstance.Execute(ctx, s.ActionConfig, pipelineContext, &s.PipelineStep)
    if err != nil {
        return fmt.Errorf("error executing action service for step %s: %w", s.ID, err)
    }

    // Store the result in the pipeline context
    if s.StepOutputKey != "" {
        pipelineContext.SetStepOutput(s.StepOutputKey, result)
    }

    return nil
}

func (s *ActionStepImpl) GetType() string {
    return "action_step"
}
