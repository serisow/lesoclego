// FetchTaxonomyAction prepares content for taxonomy operations in Drupal.
// This is a Drupal-side action that processes content from previous steps
// to prepare it for taxonomy operations. The actual taxonomy operations
// are performed in the Drupal environment.
//
// Workflow:
// 1. Collects content from required previous steps
// 2. Aggregates and formats the content
// 3. Returns the prepared content for Drupal to process
//
// The execution results will be processed by Drupal's PipelineExecutionController


package action_service

import (
	"context"
	"fmt"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

type FetchTaxonomyAction struct{}

func (a *FetchTaxonomyAction) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
    var content string
    for _, requiredStep := range requiredSteps {
        requiredStep = strings.TrimSpace(requiredStep)
        if requiredStep == "" {
            continue
        }
        stepOutput, ok := pipelineContext.GetStepOutput(requiredStep)
        if !ok {
            return "", fmt.Errorf("required content not found for fetching taxonomy: %s", requiredStep)
        }
        content += fmt.Sprintf("%v", stepOutput)
    }

    result := content

    return result, nil
}

func (a *FetchTaxonomyAction) CanHandle(actionService string) bool {
    return actionService == "fetch_taxonomy_action"
}