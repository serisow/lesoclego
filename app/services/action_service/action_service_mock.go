package action_service

import (
    "context"
    "github.com/serisow/lesocle/pipeline_type"
)

type MockActionService struct {
    Response    interface{}
    Error       error
    ServiceName string
}

func (m *MockActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    if m.Error != nil {
        return "", m.Error
    }

    switch resp := m.Response.(type) {
    case string:
        return resp, nil
    case func(context.Context, string, *pipeline_type.Context, *pipeline_type.PipelineStep) string:
        return resp(ctx, actionConfig, pipelineContext, step), nil
    default:
        return "Processed: default", nil
    }
}

func (m *MockActionService) CanHandle(actionService string) bool {
    if m.ServiceName == "" {
        return true // Default behavior for tests
    }
    return m.ServiceName == actionService
}