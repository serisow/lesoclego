package pipeline

import (
    "context"
    "fmt"
    "strings"

)

type CreateArticleAction struct{}

func (a *CreateArticleAction) Execute(ctx context.Context, actionConfig string, pipelineContext *Context, step *PipelineStep) (string, error) {
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
