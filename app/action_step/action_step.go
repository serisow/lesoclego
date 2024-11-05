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
    // Check if we have action details
    if s.PipelineStep.ActionDetails == nil {
        // Backward compatibility: treat as Drupal-side action
        s.PipelineStep.ActionDetails = &pipeline_type.ActionDetails{
            ActionService:     s.PipelineStep.ActionConfig,
            ExecutionLocation: "drupal",
            Configuration:    map[string]interface{}{},
        }
    }

    // For Drupal-side actions, just prepare the context and return
    if s.PipelineStep.ActionDetails.ExecutionLocation == "drupal" {
        if s.PipelineStep.StepOutputKey != "" {
            pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, map[string]interface{}{
                "action_config":      s.PipelineStep.ActionConfig,
                "execution_location": "drupal",
                "configuration":      s.PipelineStep.ActionDetails.Configuration,
                "action_service":     s.PipelineStep.ActionDetails.ActionService,
            })
        }
        return nil
    }

    // For Go-side actions, we need an ActionServiceInstance
    if s.ActionServiceInstance == nil {
        return fmt.Errorf("ActionService is not initialized for step %s", s.PipelineStep.ID)
    }

    result, err := s.ActionServiceInstance.Execute(ctx, s.PipelineStep.ActionConfig, pipelineContext, &s.PipelineStep)
    if err != nil {
        return fmt.Errorf("error executing action service for step %s: %w", s.PipelineStep.ID, err)
    }

    if s.PipelineStep.StepOutputKey != "" {
        pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, result)
    }

    return nil
}

func (s *ActionStepImpl) GetType() string {
    return "action_step"
}
