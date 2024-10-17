package action_service

import (
	"context"

	"github.com/serisow/lesocle/pipeline_type"
)

type MockActionService struct {
    Response string
    Error    error
}

func (m *MockActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    if m.Error != nil {
        return "", m.Error
    }
    return m.Response, nil
}