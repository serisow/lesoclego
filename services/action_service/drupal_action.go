package action_service

import (
    "context"
    "github.com/serisow/lesocle/pipeline_type"
)

// DrupalActionService handles actions that will be executed on the Drupal side
type DrupalActionService struct{}

func (d *DrupalActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    // For Drupal-side actions, we just prepare the context
    if step.StepOutputKey != "" {
        // Store the configuration that will be used by Drupal
        contextData := map[string]interface{}{
            "action_config":       actionConfig,
            "execution_location":  "drupal",
            "configuration":       step.ActionDetails.Configuration,
            "required_steps":      step.RequiredSteps,
        }
        pipelineContext.SetStepOutput(step.StepOutputKey, contextData)
    }
    return "", nil
}

func (d *DrupalActionService) CanHandle(actionService string) bool {
    // This service handles all Drupal-side actions
    return true
}

// NewDrupalActionService creates a new DrupalActionService instance
func NewDrupalActionService() *DrupalActionService {
    return &DrupalActionService{}
}