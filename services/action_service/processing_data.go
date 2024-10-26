// This is an example to demonstrate that we can perfom computationnal heavy task in the go side
// and only delagating the Drupal side, Drupal related actions like creating or updating article etc.
// This open the door to an apt plateform to do very ambitious computation and data processing
// In main.go we would add this line: 
// registry.RegisterActionService("process_data_action", &action_service.ProcessDataActionService{})
// We can create as many as we want in this folder.
// Making sure to put the correct key match the Drupal side configuration of the pipeline

package action_service

import (
    "context"
    "fmt"
    "github.com/serisow/lesocle/pipeline_type"
)

type ProcessDataActionService struct {
    BaseActionService
}

func (s *ProcessDataActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    // Validate action details are present
    if step.ActionDetails == nil {
        return "", fmt.Errorf("action details missing for step %s", step.ID)
    }

    // Use configuration from ActionDetails
    config := step.ActionDetails.Configuration
    
    // Process data using configuration
    result, err := s.processData(config)
    if err != nil {
        return "", fmt.Errorf("error processing data: %w", err)
    }

    return result, nil
}

func (s *ProcessDataActionService) CanHandle(actionService string) bool {
    return actionService == "process_data_action"
}

func (s *ProcessDataActionService) processData(config map[string]interface{}) (string, error) {
    // Implementation of data processing logic
    return "Processed data", nil
}