// pipeline/llm_step_test.go

package pipeline

import (
    "context"
    "errors"
    "os"
    "testing"

    "github.com/serisow/lesocle/pipeline/llm_service"
)

func TestLLMStepImpl_Execute(t *testing.T) {
    tests := []struct {
        name             string
        pipelineStep     PipelineStep
        pipelineContext  *Context
        mockLLMResponse  string
        mockLLMError     error
        expectedError    bool
        expectedOutput   string
    }{
        {
            name: "Successful execution with prompt placeholders",
            pipelineStep: PipelineStep{
                ID:            "llm_step_1",
                Type:          "llm_step",
                Prompt:        "Generate a summary for: {previous_step}",
                StepOutputKey: "summary",
                RequiredSteps: "previous_step",
                LLMServiceConfig: map[string]interface{}{
                    "service_name": "mock_service",
                },
            },
            pipelineContext: &Context{
                StepOutputs: map[string]interface{}{
                    "previous_step": "This is the content to summarize.",
                },
            },
            mockLLMResponse: "This is the summary.",
            expectedOutput:  "This is the summary.",
        },
        {
            name: "LLM service returns an error",
            pipelineStep: PipelineStep{
                ID:            "llm_step_2",
                Type:          "llm_step",
                Prompt:        "Generate a summary.",
                StepOutputKey: "summary",
                LLMServiceConfig: map[string]interface{}{
                    "service_name": "mock_service",
                },
            },
            pipelineContext: &Context{
                StepOutputs: make(map[string]interface{}),
            },
            mockLLMError:  errors.New("LLM service error"),
            expectedError: true,
        },
        {
            name: "Required step output missing",
            pipelineStep: PipelineStep{
                ID:            "llm_step_3",
                Type:          "llm_step",
                Prompt:        "Use the output from {missing_step}.",
                StepOutputKey: "result",
                RequiredSteps: "missing_step",
                LLMServiceConfig: map[string]interface{}{
                    "service_name": "mock_service",
                },
            },
            pipelineContext: &Context{
                StepOutputs: make(map[string]interface{}),
            },
            expectedError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create a mock LLM service
            mockLLMService := &llm_service.MockLLMService{
                CallLLMFunc: func(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
                    if tt.mockLLMError != nil {
                        return "", tt.mockLLMError
                    }
                    return tt.mockLLMResponse, nil
                },
            }

            // Initialize the LLMStepImpl with the mock service
            llmStep := &LLMStepImpl{
                PipelineStep:       tt.pipelineStep,
                LLMServiceInstance: mockLLMService,
            }

            // Execute the step
            err := llmStep.Execute(context.Background(), tt.pipelineContext)

            // Check for expected errors
            if tt.expectedError && err == nil {
                t.Errorf("Expected an error but got none")
            }
            if !tt.expectedError && err != nil {
                t.Errorf("Did not expect an error but got: %v", err)
            }

            // Verify the output in the pipeline context
            if !tt.expectedError {
                output, exists := tt.pipelineContext.GetStepOutput(tt.pipelineStep.StepOutputKey)
                if !exists {
                    t.Errorf("Expected output key '%s' not found in context", tt.pipelineStep.StepOutputKey)
                } else if output != tt.expectedOutput {
                    t.Errorf("Expected output '%s', got '%s'", tt.expectedOutput, output)
                }
            }
        })
    }
}

func TestPipelineWithLLMStep(t *testing.T) {
    // Set GO_ENVIRONMENT to "test"
    os.Setenv("GO_ENVIRONMENT", "test")

    // Mock SendExecutionResults
    originalSendExecutionResultsFunc := sendExecutionResultsFunc
    defer func() { sendExecutionResultsFunc = originalSendExecutionResultsFunc }()
    sendExecutionResultsFunc = func(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
        // Do nothing
        return nil
    }

    // Mock LLM Service
    mockLLMService := &llm_service.MockLLMService{
        CallLLMFunc: func(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
            return "LLM response based on prompt: " + prompt, nil
        },
    }

    // Setup plugin registry with mock LLM service
    registry := NewPluginRegistry()
    registry.RegisterLLMService("mock_service", mockLLMService)

    // Define pipeline steps
    steps := []PipelineStep{
        {
            ID:            "step1",
            Type:          "llm_step",
            Prompt:        "Hello, {name}!",
            StepOutputKey: "greeting",
            RequiredSteps: "name",
            LLMServiceConfig: map[string]interface{}{
                "service_name": "mock_service",
            },
        },
    }

    // Initialize pipeline context with required step outputs
    ctx := NewContext()
    ctx.SetStepOutput("name", "World")

    // Create pipeline
    p := &Pipeline{
        ID:      "test_pipeline",
        Steps:   steps,
        Context: ctx,
    }

    // Execute pipeline
    err := ExecutePipeline(p, registry)
    if err != nil {
        t.Fatalf("Pipeline execution failed: %v", err)
    }

    // Verify the output
    output, exists := ctx.GetStepOutput("greeting")
    if !exists {
        t.Fatalf("Expected output 'greeting' not found")
    }
    expectedOutput := "LLM response based on prompt: Hello, World!"
    if output != expectedOutput {
        t.Errorf("Expected output '%s', got '%s'", expectedOutput, output)
    }
}
