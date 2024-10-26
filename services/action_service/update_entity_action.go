// UpdateEntityAction prepares content for entity updates in Drupal.
// This is a Drupal-side action that processes content from a previous step
// and formats it for Drupal's entity update operations. The actual entity
// update occurs in the Drupal environment.
//
// Workflow:
// 1. Retrieves content from the specified required step
// 2. Formats the content for entity update
// 3. Returns the prepared content for Drupal to process
//
// The execution results will be processed by Drupal's PipelineExecutionController
// which handles the actual entity update using Drupal's entity API.

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

func (a *UpdateEntityAction) CanHandle(actionService string) bool {
    return actionService == "update_entity_action"
}