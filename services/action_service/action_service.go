// Package action_service provides implementations of various pipeline action services.


package action_service

import (
	"context"

	"github.com/serisow/lesocle/pipeline_type"
)


type ActionService interface {
    // Execute processes an action step
    // actionConfig is now optional as we have ActionDetails
    Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error)

    // Optional: Add a method to check if this service can handle the action
    CanHandle(actionService string) bool
}

type BaseActionService struct{}

func (b *BaseActionService) CanHandle(actionService string) bool {
    return false // Should be overridden by implementing services
}