package pipeline

import "context"


type Step interface {
	Execute(ctx context.Context, pipelineContext *Context) error

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