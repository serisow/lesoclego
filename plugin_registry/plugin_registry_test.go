package plugin_registry_test

import (
	"context"
	"testing"

	"github.com/serisow/lesocle/pipeline/step"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
	"github.com/serisow/lesocle/services/action_service"
	"github.com/serisow/lesocle/services/llm_service"
)

type MockStep struct{}

func (s *MockStep) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    return nil
}

func (s *MockStep) GetType() string {
    return "mock_step"
}

func TestRegisterAndGetStepType(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    // Register a mock step type
    registry.RegisterStepType("mock_step", func() step.Step {
        return &MockStep{}
    })

    // Retrieve the step instance
    stepInstance, err := registry.GetStepInstance("mock_step")
    if err != nil {
        t.Fatalf("Expected to retrieve step instance, got error: %v", err)
    }

    if stepInstance.GetType() != "mock_step" {
        t.Errorf("Expected step type 'mock_step', got '%s'", stepInstance.GetType())
    }
}

func TestGetUnregisteredStepType(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    _, err := registry.GetStepInstance("unknown_step")
    if err == nil {
        t.Fatal("Expected error when retrieving unregistered step type, got nil")
    }

    expectedErrorMsg := "unknown step type: unknown_step"
    if err.Error() != expectedErrorMsg {
        t.Errorf("Expected error '%s', got '%s'", expectedErrorMsg, err.Error())
    }
}

func TestRegisterAndGetLLMService(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    // Register a mock LLM service
    mockLLMService := &llm_service.MockLLMService{}
    registry.RegisterLLMService("mock_llm_service", mockLLMService)

    // Retrieve the LLM service
    service, ok := registry.GetLLMService("mock_llm_service")
    if !ok {
        t.Fatal("Expected to retrieve registered LLM service, got false")
    }

    if service != mockLLMService {
        t.Errorf("Expected retrieved service to be the same as registered service")
    }
}

func TestGetUnregisteredLLMService(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    _, ok := registry.GetLLMService("unknown_service")
    if ok {
        t.Fatal("Expected to not find unregistered LLM service, but got true")
    }
}


func TestRegisterAndGetActionService(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    // Register a mock action service
    mockActionService := &action_service.MockActionService{
        ServiceName: "process_data_action",
    }
    registry.RegisterActionService("process_data_action", mockActionService)

    // Retrieve the action service
    service, ok := registry.GetActionService("process_data_action")
    if !ok {
        t.Fatal("Expected to retrieve registered action service, got false")
    }

    if service != mockActionService {
        t.Errorf("Expected retrieved service to be the same as registered service")
    }
}

func TestGetUnregisteredActionService(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    _, ok := registry.GetActionService("unknown_service")
    if ok {
        t.Fatal("Expected to not find unregistered action service, but got true")
    }
}
