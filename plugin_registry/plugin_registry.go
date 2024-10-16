package plugin_registry

import (
	"fmt"

	"github.com/serisow/lesocle/action_service"
	"github.com/serisow/lesocle/llm_service"
	"github.com/serisow/lesocle/step"
)

type PluginRegistry struct {
    stepTypes   map[string]func() step.Step
    llmServices map[string]llm_service.LLMService
    actionServices map[string]action_service.ActionService

}

func NewPluginRegistry() *PluginRegistry {
    return &PluginRegistry{
        stepTypes:   make(map[string]func() step.Step),
        llmServices: make(map[string]llm_service.LLMService),
        actionServices: make(map[string]action_service.ActionService),
    }
}

// RegisterStepType registers a new step type
func (pr *PluginRegistry) RegisterStepType(typeName string, factory func() step.Step) {
    pr.stepTypes[typeName] = factory
}

// GetStepInstance returns a new instance of a step type
func (pr *PluginRegistry) GetStepInstance(typeName string) (step.Step, error) {
    factory, ok := pr.stepTypes[typeName]
    if !ok {
        return nil, fmt.Errorf("unknown step type: %s", typeName)
    }
    return factory(), nil
}

// RegisterLLMService registers a new LLM service
func (pr *PluginRegistry) RegisterLLMService(name string, service llm_service.LLMService) {
    pr.llmServices[name] = service
}

// GetLLMService returns an LLM service by name
func (pr *PluginRegistry) GetLLMService(name string) (llm_service.LLMService, bool) {
    service, ok := pr.llmServices[name]
    return service, ok
}

// RegisterActionService registers a new Action service
func (pr *PluginRegistry) RegisterActionService(name string, service action_service.ActionService) {
    pr.actionServices[name] = service
}

// GetActionService returns an Action service by name
func (pr *PluginRegistry) GetActionService(name string) (action_service.ActionService, bool) {
    service, ok := pr.actionServices[name]
    return service, ok
}