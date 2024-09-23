package pipeline

import (
    "context"
    "fmt"
    "strings"
)

type ActionStepImpl struct {
    PipelineStep
    result string
}

func (s *ActionStepImpl) Execute(ctx context.Context, pipelineContext *Context) error {
    var err error

    switch s.ActionConfig {
    case "create_article_action":
        err = s.createArticle(ctx, pipelineContext)
    case "update_entity_action":
        err = s.updateEntity(ctx, pipelineContext)
    case "delete_entity_action":
        err = s.deleteEntity(ctx, pipelineContext)
    case "call_api_action":
        err = s.callAPI(ctx, pipelineContext)
    default:
        return fmt.Errorf("unknown action type: %s", s.ActionConfig)
    }

    if err != nil {
        return err
    }

    // Store the result in the pipeline context
    pipelineContext.SetStepOutput(s.StepOutputKey, s.result)

    return nil
}

func (s *ActionStepImpl) createArticle(ctx context.Context, pipelineContext *Context) error {
    requiredSteps := strings.Split(s.RequiredSteps, "\r\n")
    var content string
    for _, requiredStep := range requiredSteps {
        requiredStep = strings.TrimSpace(requiredStep)
        if requiredStep == "" {
            continue
        }
        stepOutput, ok := pipelineContext.GetStepOutput(requiredStep)
        if !ok {
            return fmt.Errorf("required content not found for creating article: %s", requiredStep)
        }
        content += fmt.Sprintf("%v", stepOutput)
    }

    // Store the content as the result
    s.result = content
    return nil
}

func (s *ActionStepImpl) updateEntity(ctx context.Context, pipelineContext *Context) error {
    // Implementation for updating an entity
    s.result = "Entity updated"
    return nil
}

func (s *ActionStepImpl) deleteEntity(ctx context.Context, pipelineContext *Context) error {
    // Implementation for deleting an entity
    s.result = "Entity deleted"
    return nil
}

func (s *ActionStepImpl) callAPI(ctx context.Context, pipelineContext *Context) error {
    // Implementation for calling an external API
    s.result = "API called"
    return nil
}

func (s *ActionStepImpl) GetType() string {
    return "action_step"
}