package action_service

import (
	"context"

	"github.com/serisow/lesocle/pipeline_type"
)

type ActionService interface {
    Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error)
}
