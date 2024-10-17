package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/serisow/lesocle/action_step"
	"github.com/serisow/lesocle/llm_step"
	"github.com/serisow/lesocle/pipeline"
	"github.com/serisow/lesocle/pipeline/step"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
	"github.com/serisow/lesocle/services/action_service"
	"github.com/serisow/lesocle/services/llm_service"
)

// Mock implementations for testing

type MockLLMService struct {
    Response string
    Error    error
}

func (m *MockLLMService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    if m.Error != nil {
        return "", m.Error
    }
    return m.Response, nil
}



type MockGoogleSearchStep struct {
	PipelineStep pipeline_type.PipelineStep
    Response string
    Error    error
}

func (s *MockGoogleSearchStep) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    if s.Error != nil {
        return s.Error
    }
    if s.PipelineStep.StepOutputKey != "" {
        pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, s.Response)
    }
    return nil
}

func (s *MockGoogleSearchStep) GetType() string {
    return "google_search"
}

func TestFullPipelineExecution(t *testing.T) {
    // Set GO_ENVIRONMENT to "test"
    os.Setenv("GO_ENVIRONMENT", "test")

    // Mock SendExecutionResults
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Do nothing
        return nil
    }

    // Initialize the plugin registry
    registry := plugin_registry.NewPluginRegistry()

    // Register mock LLM service
    mockLLMService := &MockLLMService{Response: "LLM step output"}
    registry.RegisterLLMService("mock_llm_service", mockLLMService)

    // Register mock action service
    mockActionService := &action_service.MockActionService{Response: "Action step output"}
    registry.RegisterActionService("mock_action_service", mockActionService)

    // Register mock Google search step
    registry.RegisterStepType("google_search", func() step.Step {
        return &MockGoogleSearchStep{Response: "Google search output"}
    })

	// Register llm_step
	registry.RegisterStepType("llm_step", func() step.Step {
		return &llm_step.LLMStepImpl{}
	})

	// Register action_step
	registry.RegisterStepType("action_step", func() step.Step {
		return &action_step.ActionStepImpl{}
	})


    // Define pipeline steps
    steps := []pipeline_type.PipelineStep{
        {
            ID:            "llm_step_1",
            Type:          "llm_step",
            Prompt:        "This is a test prompt.",
            StepOutputKey: "llm_output",
            LLMServiceConfig: map[string]interface{}{
                "service_name": "mock_llm_service",
            },
        },
        {
            ID:            "google_search_1",
            Type:          "google_search",
            StepOutputKey: "search_output",
            // Additional configuration if needed
        },
        {
            ID:            "action_step_1",
            Type:          "action_step",
            ActionConfig:  "mock_action_service",
            RequiredSteps: "llm_output\r\nsearch_output",
            StepOutputKey: "action_output",
        },
    }

    // Initialize pipeline context
    ctx := pipeline_type.NewContext()

    // Create pipeline
    p := &pipeline_type.Pipeline{
        ID:      "test_pipeline",
        Steps:   steps,
        Context: ctx,
    }

    // Execute pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err != nil {
        t.Fatalf("Pipeline execution failed: %v", err)
    }

    // Verify the outputs
    output, exists := ctx.GetStepOutput("llm_output")
    if !exists || output != "LLM step output" {
        t.Errorf("Expected LLM output 'LLM step output', got '%v'", output)
    }

    output, exists = ctx.GetStepOutput("search_output")
    if !exists || output != "Google search output" {
        t.Errorf("Expected search output 'Google search output', got '%v'", output)
    }

    output, exists = ctx.GetStepOutput("action_output")
    if !exists || output != "Action step output" {
        t.Errorf("Expected action output 'Action step output', got '%v'", output)
    }
}

func TestPipelineExecutionWithErrorHandling(t *testing.T) {
    // Set GO_ENVIRONMENT to "test"
    os.Setenv("GO_ENVIRONMENT", "test")

    // Mock SendExecutionResults
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Do nothing
        return nil
    }

    // Initialize the plugin registry
    registry := plugin_registry.NewPluginRegistry()

    // Register mock LLM service that returns an error
    mockLLMService := &MockLLMService{Error: errors.New("Mock LLM error")}
    registry.RegisterLLMService("mock_llm_service", mockLLMService)

	// Register llm_step
	registry.RegisterStepType("llm_step", func() step.Step {
		return &llm_step.LLMStepImpl{}
	})

	// Register action_step
	registry.RegisterStepType("action_step", func() step.Step {
		return &action_step.ActionStepImpl{}
	})
	

    // Define pipeline steps
    steps := []pipeline_type.PipelineStep{
        {
            ID:            "llm_step_1",
            Type:          "llm_step",
            Prompt:        "This is a test prompt.",
            StepOutputKey: "llm_output",
            LLMServiceConfig: map[string]interface{}{
                "service_name": "mock_llm_service",
            },
        },
    }

    // Initialize pipeline context
    ctx := pipeline_type.NewContext()

    // Create pipeline
    p := &pipeline_type.Pipeline{
        ID:      "test_pipeline_error",
        Steps:   steps,
        Context: ctx,
    }

    // Execute pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err == nil {
        t.Fatal("Expected pipeline execution to fail, but it succeeded")
    }

    expectedErrorMsg := "error calling LLM service for step llm_step_1: Mock LLM error"
    if err.Error() != expectedErrorMsg {
        t.Errorf("Expected error '%s', got '%s'", expectedErrorMsg, err.Error())
    }
}

func TestPipelineLLMToActionIntegration(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    // Mock LLM and Action Services
    mockLLMService := &llm_service.MockLLMService{
        CallLLMFunc: func(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
            return "Generated content", nil
        },
    }
    mockActionService := &action_service.CreateArticleAction{}

    // Register Services in Registry
    registry.RegisterLLMService("mock_llm_service", mockLLMService)
    registry.RegisterActionService("create_article_action", mockActionService)

    // Register Step Types
    registry.RegisterStepType("llm_step", func() step.Step {
        return &llm_step.LLMStepImpl{}
    })
    registry.RegisterStepType("action_step", func() step.Step {
        return &action_step.ActionStepImpl{}
    })

    // Mock SendExecutionResults to avoid actual HTTP calls
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Mock implementation; do nothing
        return nil
    }

    // Define Pipeline Steps
    steps := []pipeline_type.PipelineStep{
        {
            ID: "llm_step_1",
            Type: "llm_step",
            StepOutputKey: "content",
            Prompt: "Generate a topic article",
            LLMServiceConfig: map[string]interface{}{"service_name": "mock_llm_service"},
        },
        {
            ID: "action_step_1",
            Type: "action_step",
            RequiredSteps: "content",
            ActionConfig: "create_article_action",
            StepOutputKey: "final_article",
        },
    }

    // Initialize Context and Pipeline
    ctx := pipeline_type.NewContext()
    p := &pipeline_type.Pipeline{ID: "test_integration_pipeline", Steps: steps, Context: ctx}

    // Execute the Pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err != nil {
        t.Fatalf("Pipeline execution failed: %v", err)
    }

    // Validate Output
    output, exists := ctx.GetStepOutput("final_article")
    if !exists || output != "Generated content" {
        t.Errorf("Expected 'Generated content', got '%v'", output)
    }
}

func TestPipelineComplexStepSequenceIntegration(t *testing.T) {
    registry := plugin_registry.NewPluginRegistry()

    // Mock LLM and Action Services
    mockLLMService := &llm_service.MockLLMService{
        CallLLMFunc: func(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
            // Custom response based on prompt to simulate realistic behavior
            if prompt == "Generate intro content" {
                return "This is the intro content.", nil
            } else if prompt == "Refine the article" {
                return "This is the refined article.", nil
            }
            return "Default response", nil
        },
    }
    mockActionService := &action_service.CreateArticleAction{}

    // Register Services in Registry
    registry.RegisterLLMService("mock_llm_service", mockLLMService)
    registry.RegisterActionService("create_article_action", mockActionService)
    registry.RegisterActionService("update_entity_action", &action_service.UpdateEntityAction{})

    // Register Step Types
    registry.RegisterStepType("llm_step", func() step.Step {
        return &llm_step.LLMStepImpl{}
    })
    registry.RegisterStepType("action_step", func() step.Step {
        return &action_step.ActionStepImpl{}
    })

    // Mock SendExecutionResults to avoid actual HTTP calls
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Mock implementation; do nothing
        return nil
    }

    // Define Pipeline Steps
    steps := []pipeline_type.PipelineStep{
        {
            ID: "llm_step_1",
            Type: "llm_step",
            StepOutputKey: "intro_content",
            Prompt: "Generate intro content",
            LLMServiceConfig: map[string]interface{}{"service_name": "mock_llm_service"},
        },
        {
            ID: "action_step_1",
            Type: "action_step",
            RequiredSteps: "intro_content",
            ActionConfig: "create_article_action",
            StepOutputKey: "article",
        },
        {
            ID: "llm_step_2",
            Type: "llm_step",
            StepOutputKey: "refined_article",
            Prompt: "Refine the article",
            RequiredSteps: "article",
            LLMServiceConfig: map[string]interface{}{"service_name": "mock_llm_service"},
        },
        {
            ID: "action_step_2",
            Type: "action_step",
            RequiredSteps: "refined_article",
            ActionConfig: "update_entity_action",
            StepOutputKey: "final_article",
        },
    }

    // Initialize Context and Pipeline
    ctx := pipeline_type.NewContext()
    p := &pipeline_type.Pipeline{ID: "test_complex_pipeline", Steps: steps, Context: ctx}

    // Execute the Pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err != nil {
        t.Fatalf("Pipeline execution failed: %v", err)
    }

    // Validate Output
    finalArticle, exists := ctx.GetStepOutput("final_article")
    if !exists || finalArticle != "This is the refined article." {
        t.Errorf("Expected 'This is the refined article.', got '%v'", finalArticle)
    }
}


func TestPipelineExecutionWithActionServiceError(t *testing.T) {
    // Set GO_ENVIRONMENT to "test"
    os.Setenv("GO_ENVIRONMENT", "test")

    // Mock SendExecutionResults
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Do nothing
        return nil
    }

    // Initialize the plugin registry
    registry := plugin_registry.NewPluginRegistry()

    // Register mock action service that returns an error
    mockActionService := &action_service.MockActionService{Error: errors.New("Mock Action Service error")}
    registry.RegisterActionService("mock_action_service", mockActionService)

    // Register action_step
    registry.RegisterStepType("action_step", func() step.Step {
        return &action_step.ActionStepImpl{}
    })

    // Define pipeline steps
    steps := []pipeline_type.PipelineStep{
        {
            ID:            "action_step_1",
            Type:          "action_step",
            ActionConfig:  "mock_action_service",
            StepOutputKey: "action_output",
        },
    }

    // Initialize pipeline context
    ctx := pipeline_type.NewContext()

    // Create pipeline
    p := &pipeline_type.Pipeline{
        ID:      "test_pipeline_action_error",
        Steps:   steps,
        Context: ctx,
    }

    // Execute pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err == nil {
        t.Fatal("Expected pipeline execution to fail, but it succeeded")
    }

    expectedErrorMsg := "error executing action service for step action_step_1: Mock Action Service error"
    if err.Error() != expectedErrorMsg {
        t.Errorf("Expected error '%s', got '%s'", expectedErrorMsg, err.Error())
    }
}


func TestSendExecutionResults_Success(t *testing.T) {
    // Set up a mock server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request method and headers
        if r.Method != http.MethodPost {
            t.Errorf("Expected method POST, got %s", r.Method)
        }
        if r.Header.Get("Content-Type") != "application/json" {
            t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
        }

        // Read and verify the request body
        body, err := io.ReadAll(r.Body)
        if err != nil {
            t.Errorf("Error reading request body: %v", err)
        }
        defer r.Body.Close()

        var data map[string]interface{}
        err = json.Unmarshal(body, &data)
        if err != nil {
            t.Errorf("Error unmarshaling JSON: %v", err)
        }

        // Verify the pipeline ID and results in the request body
        if data["pipeline_id"] != "test_pipeline" {
            t.Errorf("Expected pipeline_id 'test_pipeline', got '%v'", data["pipeline_id"])
        }

        // Respond with 200 OK
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    // Override the API endpoint in config
    originalAPIEndpoint := os.Getenv("API_ENDPOINT")
    defer os.Setenv("API_ENDPOINT", originalAPIEndpoint)
    os.Setenv("API_ENDPOINT", server.URL)

    // Prepare test data
    pipelineID := "test_pipeline"
    results := map[string]interface{}{
        "step1": map[string]interface{}{
            "status": "completed",
            "data":   "test data",
        },
    }
    startTime := time.Now().Unix()
    endTime := startTime + 10

    // Call the function
    err := pipeline.SendExecutionResults(pipelineID, results, startTime, endTime)
    if err != nil {
        t.Errorf("Expected no error, got %v", err)
    }
}

func TestSendExecutionResults_Non200Response(t *testing.T) {
    // Set up a mock server that responds with 500 Internal Server Error
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer server.Close()

    // Override the API endpoint in config
    originalAPIEndpoint := os.Getenv("API_ENDPOINT")
    defer os.Setenv("API_ENDPOINT", originalAPIEndpoint)
    os.Setenv("API_ENDPOINT", server.URL)

    // Prepare test data
    pipelineID := "test_pipeline"
    results := map[string]interface{}{}
    startTime := time.Now().Unix()
    endTime := startTime + 10

    // Call the function
    err := pipeline.SendExecutionResults(pipelineID, results, startTime, endTime)
    if err == nil {
        t.Errorf("Expected error due to non-200 response, got nil")
    } else {
        expectedErrorMsg := "unexpected status code: 500"
        if err.Error() != expectedErrorMsg {
            t.Errorf("Expected error '%s', got '%s'", expectedErrorMsg, err.Error())
        }
    }
}

func TestSendExecutionResults_MarshalError(t *testing.T) {
    // Prepare test data with unmarshalable data
    pipelineID := "test_pipeline"
    results := map[string]interface{}{
        "invalid": make(chan int), // channels cannot be marshaled to JSON
    }
    startTime := time.Now().Unix()
    endTime := startTime + 10

    // Call the function
    err := pipeline.SendExecutionResults(pipelineID, results, startTime, endTime)
    if err == nil {
        t.Errorf("Expected error due to JSON marshal error, got nil")
    } else {
        if !strings.Contains(err.Error(), "error marshaling results") {
            t.Errorf("Expected marshal error, got '%s'", err.Error())
        }
    }
}

func TestSendExecutionResults_NetworkError(t *testing.T) {
    // Close the server to simulate a network error
    server := httptest.NewServer(nil)
    server.Close()

    // Override the API endpoint in config to point to the closed server
    originalAPIEndpoint := os.Getenv("API_ENDPOINT")
    defer os.Setenv("API_ENDPOINT", originalAPIEndpoint)
    os.Setenv("API_ENDPOINT", server.URL)

    // Prepare test data
    pipelineID := "test_pipeline"
    results := map[string]interface{}{}
    startTime := time.Now().Unix()
    endTime := startTime + 10

    // Call the function
    err := pipeline.SendExecutionResults(pipelineID, results, startTime, endTime)
    if err == nil {
        t.Errorf("Expected network error, got nil")
    } else {
        if !strings.Contains(err.Error(), "error sending results") {
            t.Errorf("Expected network error, got '%s'", err.Error())
        }
    }
}

func TestPipelineExecutionWithUnknownStepType(t *testing.T) {
    // Set GO_ENVIRONMENT to "test"
    os.Setenv("GO_ENVIRONMENT", "test")

    // Mock SendExecutionResults
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Do nothing
        return nil
    }

    // Initialize the plugin registry without registering any steps
    registry := plugin_registry.NewPluginRegistry()

    // Define pipeline steps with an unknown step type
    steps := []pipeline_type.PipelineStep{
        {
            ID:   "unknown_step_1",
            Type: "unknown_step",
        },
    }

    // Initialize pipeline context
    ctx := pipeline_type.NewContext()

    // Create pipeline
    p := &pipeline_type.Pipeline{
        ID:      "test_pipeline_unknown_step",
        Steps:   steps,
        Context: ctx,
    }

    // Execute pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err == nil {
        t.Fatal("Expected pipeline execution to fail due to unknown step type, but it succeeded")
    }

    expectedErrorMsg := "unknown step type: unknown_step"
    if err.Error() != expectedErrorMsg {
        t.Errorf("Expected error '%s', got '%s'", expectedErrorMsg, err.Error())
    }
}

// StepWithoutPipelineStepField simulates a step that lacks the PipelineStep field
type StepWithoutPipelineStepField struct{}

func (s *StepWithoutPipelineStepField) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    return nil
}

func (s *StepWithoutPipelineStepField) GetType() string {
    return "no_pipeline_step_field"
}

func TestPipelineExecutionWithReflectionFailure(t *testing.T) {
    // Set GO_ENVIRONMENT to "test"
    os.Setenv("GO_ENVIRONMENT", "test")

    // Mock SendExecutionResults
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Do nothing
        return nil
    }

    // Initialize the plugin registry and register the faulty step
    registry := plugin_registry.NewPluginRegistry()
    registry.RegisterStepType("no_pipeline_step_field", func() step.Step {
        return &StepWithoutPipelineStepField{}
    })

    // Define pipeline steps
    steps := []pipeline_type.PipelineStep{
        {
            ID:   "step1",
            Type: "no_pipeline_step_field",
        },
    }

    // Initialize pipeline context
    ctx := pipeline_type.NewContext()

    // Create pipeline
    p := &pipeline_type.Pipeline{
        ID:      "test_pipeline_reflection_failure",
        Steps:   steps,
        Context: ctx,
    }

    // Execute pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err == nil {
        t.Fatal("Expected pipeline execution to fail due to reflection error, but it succeeded")
    }

    expectedErrorMsg := "cannot set PipelineStep for step type no_pipeline_step_field: field PipelineStep not found"
    if err.Error() != expectedErrorMsg {
        t.Errorf("Expected error '%s', got '%s'", expectedErrorMsg, err.Error())
    }
}

func TestPipelineExecutionWithStepInitializationError(t *testing.T) {
    // Set GO_ENVIRONMENT to "test"
    os.Setenv("GO_ENVIRONMENT", "test")

    // Mock SendExecutionResults
    originalSendExecutionResultsFunc := pipeline.SendExecutionResultsFunc
    defer func() { pipeline.SendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    pipeline.SendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Do nothing
        return nil
    }

    // Initialize the plugin registry and register a step that requires configuration
    registry := plugin_registry.NewPluginRegistry()

    // Register llm_step
    registry.RegisterStepType("llm_step", func() step.Step {
        return &llm_step.LLMStepImpl{}
    })

    // Define pipeline steps with missing LLMServiceConfig
    steps := []pipeline_type.PipelineStep{
        {
            ID:            "llm_step_1",
            Type:          "llm_step",
            Prompt:        "Test prompt",
            StepOutputKey: "output",
            // LLMServiceConfig is intentionally missing
        },
    }

    // Initialize pipeline context
    ctx := pipeline_type.NewContext()

    // Create pipeline
    p := &pipeline_type.Pipeline{
        ID:      "test_pipeline_step_init_error",
        Steps:   steps,
        Context: ctx,
    }

    // Execute pipeline
    err := pipeline.ExecutePipeline(p, registry)
    if err == nil {
        t.Fatal("Expected pipeline execution to fail due to missing configuration, but it succeeded")
    }

    expectedErrorMsg := "service_name not found in llm_service configuration for step llm_step_1"
    if err.Error() != expectedErrorMsg {
        t.Errorf("Expected error '%s', got '%s'", expectedErrorMsg, err.Error())
    }
}
