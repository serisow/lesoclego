package action_step

import (
	"context"
	"testing"

	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
	"github.com/serisow/lesocle/services/action_service"
)

// in action_step_test.go
func TestActionStepImpl_Execute(t *testing.T) {
    tests := []struct {
        name             string
        actionConfig     string
        requiredSteps    string
        stepOutputKey    string
        actionDetails    *pipeline_type.ActionDetails
        goSideService    bool // Add this flag to indicate if we need to register a Go-side service
        pipelineContext  *pipeline_type.Context
        expectedOutput   map[string]interface{}
        expectedError    bool
        expectedErrorMsg string
    }{
        {
            name:          "Drupal-side create article action",
            actionConfig:  "create_article_action",
            requiredSteps: "step1",
            stepOutputKey: "article_content",
            actionDetails: &pipeline_type.ActionDetails{
                ActionService:      "create_article_action",
                ExecutionLocation: "drupal",
                Configuration: map[string]interface{}{
                    "required_steps": []string{"step1"},
                },
            },
            goSideService: false,
            pipelineContext: func() *pipeline_type.Context {
                ctx := pipeline_type.NewContext()
                ctx.SetStepOutput("step1", "Article body content.")
                return ctx
            }(),
            expectedOutput: map[string]interface{}{
                "action_config":      "create_article_action",
                "execution_location": "drupal",
                "action_service":     "create_article_action",
                "configuration": map[string]interface{}{
                    "required_steps": []string{"step1"},
                },
            },
            expectedError: false,
        },
        {
            name:          "Go-side process data action",
            actionConfig:  "process_data_action",
            requiredSteps: "step1",
            stepOutputKey: "processed_data",
            actionDetails: &pipeline_type.ActionDetails{
                ActionService:      "process_data_action",
                ExecutionLocation: "go",
                Configuration: map[string]interface{}{},
            },
            goSideService: true,
            pipelineContext: func() *pipeline_type.Context {
                ctx := pipeline_type.NewContext()
                ctx.SetStepOutput("step1", "Data to process")
                return ctx
            }(),
            expectedOutput: map[string]interface{}{
                "processed_data": "Processed: Data to process",
            },
            expectedError: false,
        },
        {
            name:          "Go-side action without service",
            actionConfig:  "missing_action",
            requiredSteps: "",
            stepOutputKey: "output",
            actionDetails: &pipeline_type.ActionDetails{
                ActionService:      "missing_action",
                ExecutionLocation: "go",
                Configuration: map[string]interface{}{},
            },
            goSideService: false,
            expectedError:    true,
            expectedErrorMsg: "ActionService is not initialized for step test_step",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Initialize the registry
            registry := plugin_registry.NewPluginRegistry()

            // Only register action service for Go-side tests that need it
            if tt.goSideService {
				mockActionService := &action_service.MockActionService{
					ServiceName: tt.actionConfig,
					Response: func(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) string {
						// Get the required step's output and process it
						if output, exists := pipelineContext.GetStepOutput("step1"); exists {
							return "Processed: " + output.(string)
						}
						return "Processed: default"
					},
				}
                registry.RegisterActionService(tt.actionConfig, mockActionService)
            }

            actionStep := &ActionStepImpl{
                PipelineStep: pipeline_type.PipelineStep{
                    ID:            "test_step",
                    ActionConfig:  tt.actionConfig,
                    RequiredSteps: tt.requiredSteps,
                    StepOutputKey: tt.stepOutputKey,
                    ActionDetails: tt.actionDetails,
                },
            }

            // Set ActionServiceInstance for Go-side actions
            if tt.goSideService {
                if service, ok := registry.GetActionService(tt.actionConfig); ok {
                    actionStep.ActionServiceInstance = service
                }
            }

            // Ensure pipelineContext is initialized
            if tt.pipelineContext == nil {
                tt.pipelineContext = pipeline_type.NewContext()
            }

            // Execute the action step
            err := actionStep.Execute(context.Background(), tt.pipelineContext)

            // Check for expected errors
            if tt.expectedError {
                if err == nil {
                    t.Errorf("Expected an error but got none")
                } else if tt.expectedErrorMsg != "" && err.Error() != tt.expectedErrorMsg {
                    t.Errorf("Expected error '%s', got '%s'", tt.expectedErrorMsg, err.Error())
                }
                return
            }

            if err != nil {
                t.Errorf("Did not expect an error but got: %v", err)
                return
            }

            // Verify the output in the pipeline context
            output, exists := tt.pipelineContext.GetStepOutput(tt.stepOutputKey)
            if !exists {
                t.Errorf("Expected output key '%s' not found in context", tt.stepOutputKey)
                return
            }

            // For Drupal-side actions, verify the preparation of context
            if tt.actionDetails.ExecutionLocation == "drupal" {
                actionOutput, ok := output.(map[string]interface{})
                if !ok {
                    t.Errorf("Expected output to be map[string]interface{}, got %T", output)
                    return
                }

                // Verify each expected key in the output
                for key, expectedValue := range tt.expectedOutput {
                    actualValue, exists := actionOutput[key]
                    if !exists {
                        t.Errorf("Expected key '%s' not found in output", key)
                        continue
                    }

                    // Deep comparison for nested maps
                    if configMap, ok := expectedValue.(map[string]interface{}); ok {
                        actualConfigMap, ok := actualValue.(map[string]interface{})
                        if !ok {
                            t.Errorf("Expected %s to be map[string]interface{}, got %T", key, actualValue)
                            continue
                        }
                        compareConfigMaps(t, configMap, actualConfigMap, key)
                    } else if actualValue != expectedValue {
                        t.Errorf("Expected %s to be '%v', got '%v'", key, expectedValue, actualValue)
                    }
                }
            } else {
                // For Go-side actions, verify the direct output
                if output != tt.expectedOutput["processed_data"] {
                    t.Errorf("Expected output '%v', got '%v'", tt.expectedOutput["processed_data"], output)
                }
            }
        })
    }
}

// Helper function to compare nested configuration maps
func compareConfigMaps(t *testing.T, expected, actual map[string]interface{}, path string) {
    for key, expectedValue := range expected {
        actualValue, exists := actual[key]
        if !exists {
            t.Errorf("Expected key '%s' not found in %s", key, path)
            continue
        }

        switch v := expectedValue.(type) {
        case map[string]interface{}:
            actualMap, ok := actualValue.(map[string]interface{})
            if !ok {
                t.Errorf("Expected %s.%s to be map[string]interface{}, got %T", path, key, actualValue)
                continue
            }
            compareConfigMaps(t, v, actualMap, path+"."+key)
        case []string:
            actualSlice, ok := actualValue.([]string)
            if !ok {
                t.Errorf("Expected %s.%s to be []string, got %T", path, key, actualValue)
                continue
            }
            if len(v) != len(actualSlice) {
                t.Errorf("Expected %s.%s to have length %d, got %d", path, key, len(v), len(actualSlice))
                continue
            }
            for i, expectedStr := range v {
                if actualSlice[i] != expectedStr {
                    t.Errorf("Expected %s.%s[%d] to be '%s', got '%s'", path, key, i, expectedStr, actualSlice[i])
                }
            }
        default:
            if actualValue != expectedValue {
                t.Errorf("Expected %s.%s to be '%v', got '%v'", path, key, expectedValue, actualValue)
            }
        }
    }
}
