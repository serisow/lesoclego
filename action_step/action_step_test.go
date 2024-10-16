package action_step

import (
	"context"
	"testing"

	"github.com/serisow/lesocle/services/action_service"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
)

// pipeline/action_step_test.go

func TestActionStepImpl_Execute(t *testing.T) {
	tests := []struct {
		name             string
		actionConfig     string
		requiredSteps    string
		stepOutputKey    string
		pipelineContext  *pipeline_type.Context
		expectedResult   string
		expectedError    bool
		expectedErrorMsg string
	}{
		{
			name:          "Successful create article action",
			actionConfig:  "create_article_action",
			requiredSteps: "step1",
			stepOutputKey: "article_content",
			pipelineContext: func() *pipeline_type.Context {
				ctx := pipeline_type.NewContext()
				ctx.SetStepOutput("step1", "Article body content.")
				return ctx
			}(),
			expectedResult: "Article body content.",
		},
		// ... other test cases ...
	}

	// Initialize the plugin registry and register action services
	registry := plugin_registry.NewPluginRegistry()
	registry.RegisterActionService("create_article_action", &action_service.CreateArticleAction{})
	registry.RegisterActionService("update_entity_action", &action_service.UpdateEntityAction{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actionStep := &ActionStepImpl{
				PipelineStep: pipeline_type.PipelineStep{
					ActionConfig:  tt.actionConfig,
					RequiredSteps: tt.requiredSteps,
					StepOutputKey: tt.stepOutputKey,
				},
			}

			actionServiceInstance, ok := registry.GetActionService(tt.actionConfig)
			if !ok && !tt.expectedError {
				t.Fatalf("Action service '%s' not registered", tt.actionConfig)
			}

			actionStep.ActionServiceInstance = actionServiceInstance

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
				} else if err.Error() != tt.expectedErrorMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedErrorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect an error but got: %v", err)
				} else {
					// Verify the result in the pipeline context
					output, exists := tt.pipelineContext.GetStepOutput(tt.stepOutputKey)
					if !exists {
						t.Errorf("Expected output key '%s' not found in context", tt.stepOutputKey)
					} else if output != tt.expectedResult {
						t.Errorf("Expected output '%s', got '%s'", tt.expectedResult, output)
					}
				}
			}
		})
	}
}
