package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ActionStepImpl struct {
    PipelineStep
}

func (s *ActionStepImpl) Execute(ctx context.Context, pipelineContext *Context) error {
    // Implementation based on action type
    switch s.ActionConfig {
    case "create_article_action":
        return s.createArticle(ctx, pipelineContext)
    case "update_entity_action":
        return s.updateEntity(ctx, pipelineContext)
    case "delete_entity_action":
        return s.deleteEntity(ctx, pipelineContext)
    case "call_api_action":
        return s.callAPI(ctx, pipelineContext)
    default:
        return fmt.Errorf("unknown action type: %s", s.ActionConfig)
    }
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

    // Create a filename with a timestamp
    timestamp := time.Now().Format("20060102_150405")
    filename := filepath.Join("output", fmt.Sprintf("article_%s.txt", timestamp))

    // Ensure the output directory exists
    err := os.MkdirAll("output", os.ModePerm)
    if err != nil {
        return fmt.Errorf("failed to create output directory: %w", err)
    }

    // Write the content to the file
    err = os.WriteFile(filename, []byte(content), 0644)
    if err != nil {
        return fmt.Errorf("failed to write article to file: %w", err)
    }

    fmt.Printf("Article created and saved to: %s\n", filename)

    return nil
}


func (s *ActionStepImpl) updateEntity(ctx context.Context, pipelineContext *Context) error {
    // Implementation for updating an entity
    return fmt.Errorf("updateEntity not implemented")
}

func (s *ActionStepImpl) deleteEntity(ctx context.Context, pipelineContext *Context) error {
    // Implementation for deleting an entity
    return fmt.Errorf("deleteEntity not implemented")
}

func (s *ActionStepImpl) callAPI(ctx context.Context, pipelineContext *Context) error {
    // Implementation for calling an external API
    return fmt.Errorf("callAPI not implemented")
}

func (s *ActionStepImpl) GetType() string {
    return "action_step"
}