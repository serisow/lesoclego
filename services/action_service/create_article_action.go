// CreateArticleAction prepares content for article creation in Drupal.
// This is a Drupal-side action service that aggregates content from previous pipeline
// steps and formats it for Drupal's article creation process. The actual article
// creation happens in the Drupal environment during the final execution phase.
//
// Workflow:
// 1. Collects content from required previous steps
// 2. Aggregates and formats the content
// 3. Returns the prepared content for Drupal to process
//
// The execution results will be processed by Drupal's PipelineExecutionController
// which handles the actual article creation using Drupal's entity API.


package action_service

import (
	"context"
	"fmt"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

type CreateArticleAction struct{}

func (a *CreateArticleAction) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
    var content string
    for _, requiredStep := range requiredSteps {
        requiredStep = strings.TrimSpace(requiredStep)
        if requiredStep == "" {
            continue
        }
        stepOutput, ok := pipelineContext.GetStepOutput(requiredStep)
        if !ok {
            return "", fmt.Errorf("required content not found for creating article: %s", requiredStep)
        }
        content += fmt.Sprintf("%v", stepOutput)
    }

    // Here you can add logic to create the article using `content`.
    result := content // For simplicity, we'll just return the content.

    return result, nil
}

func (a *CreateArticleAction) CanHandle(actionService string) bool {
    return actionService == "create_article_action"
}
