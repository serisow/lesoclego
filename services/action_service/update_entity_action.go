package action_service

import (
	"context"
	"fmt"

	"github.com/serisow/lesocle/pipeline_type"
)

type UpdateEntityAction struct{}

func (a *UpdateEntityAction) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    // Retrieve refined content from the required step output
    if output, exists := pipelineContext.GetStepOutput(step.RequiredSteps); exists {
        return fmt.Sprintf("%v", output), nil
    }
    return "", fmt.Errorf("required refined content not found")
}

