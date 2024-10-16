package pipeline

import (
	"context"

)

type ActionService interface {
    Execute(ctx context.Context, actionConfig string, pipelineContext *Context, step *PipelineStep) (string, error)
}
