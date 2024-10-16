package step

import (
	"context"

	"github.com/serisow/lesocle/pipeline_type"
)


type Step interface {
	Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error

    GetType() string
}

type LLMStep interface {
    Step
    CallLLM(ctx context.Context, prompt string, config map[string]interface{}) (string, error)
}

type ActionStep interface {
    Step
    PerformAction(config map[string]interface{}) error
}

type GoogleSearchStep interface {
    Step
    // Add any specific methods for Google search if needed
}