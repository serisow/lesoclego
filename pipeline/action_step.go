package pipeline

import (
    "context"
    "fmt"

)

type ActionStepImpl struct {
    PipelineStep
    ActionServiceInstance ActionService
}

func (s *ActionStepImpl) Execute(ctx context.Context, pipelineContext *Context) error {
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
